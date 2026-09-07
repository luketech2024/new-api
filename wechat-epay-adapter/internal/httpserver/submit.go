package httpserver

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/epay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const cashierCookiePrefix = "epay_cashier_"

type SubmitHandler struct {
	store           *store.Store
	partnerID       string
	key             string
	notifyURLPolicy *order.NotifyURLPolicy
	wechatNotifyURL string
	maximumAmount   string
	returnURLPolicy *order.ReturnURLPolicy
	nativeOrders    *order.NativeOrderService
}

func NewSubmitHandler(store *store.Store, appConfig config.Config, returnURLPolicy *order.ReturnURLPolicy, notifyURLPolicy *order.NotifyURLPolicy, nativeOrders ...*order.NativeOrderService) *SubmitHandler {
	handler := &SubmitHandler{
		store: store, partnerID: appConfig.EpayPartnerID, key: appConfig.EpayKey,
		notifyURLPolicy: notifyURLPolicy, wechatNotifyURL: appConfig.WechatNotifyURL,
		maximumAmount: appConfig.MaxOrderAmountYuan, returnURLPolicy: returnURLPolicy,
	}
	if len(nativeOrders) > 0 {
		handler.nativeOrders = nativeOrders[0]
	}
	return handler
}

func (handler *SubmitHandler) Handle(context *gin.Context) {
	if err := context.Request.ParseForm(); err != nil {
		AbortErrorPage(context, http.StatusBadRequest, "form body is not parseable: "+err.Error())
		return
	}
	if len(context.Request.PostForm) > 10 || hasDuplicateField(context.Request.PostForm) {
		AbortErrorPage(context, http.StatusBadRequest, fmt.Sprintf("unexpected form shape: %d fields, duplicates=%t", len(context.Request.PostForm), hasDuplicateField(context.Request.PostForm)))
		return
	}
	params := make(map[string]string, len(context.Request.PostForm))
	for name, values := range context.Request.PostForm {
		params[name] = values[0]
	}
	request := SubmitRequest{
		PartnerID: params["pid"], PaymentType: params["type"], MerchantOrder: params["out_trade_no"],
		NotifyURL: params["notify_url"], ReturnURL: params["return_url"], Subject: params["name"], Money: params["money"],
		Device: params["device"], SignType: params["sign_type"], Signature: params["sign"],
	}
	if request.PartnerID == "" || request.PaymentType == "" || request.MerchantOrder == "" || request.NotifyURL == "" || request.ReturnURL == "" || request.Subject == "" || request.Money == "" || request.SignType == "" || request.Signature == "" {
		AbortErrorPage(context, http.StatusBadRequest, "required epay field missing")
		return
	}
	if request.PartnerID != handler.partnerID || request.PaymentType != epay.PaymentTypeWechat || request.SignType != epay.SignTypeMD5 || !epay.Verify(params, handler.key) {
		AbortErrorPage(context, http.StatusForbidden, fmt.Sprintf("credentials rejected: pid_match=%t type=%q sign_type=%q signature_valid=%t", request.PartnerID == handler.partnerID, request.PaymentType, request.SignType, epay.Verify(params, handler.key)))
		return
	}
	if err := order.ValidateMerchantOrder(request.MerchantOrder); err != nil {
		AbortErrorPage(context, http.StatusBadRequest, "merchant order rejected: "+err.Error())
		return
	}
	if len(request.Subject) > 128 || len(request.NotifyURL) > 2048 || len(request.ReturnURL) > 2048 {
		AbortErrorPage(context, http.StatusBadRequest, fmt.Sprintf("field too long: subject=%d notify_url=%d return_url=%d", len(request.Subject), len(request.NotifyURL), len(request.ReturnURL)))
		return
	}
	amountFen, amountText, err := order.ParseAmountFen(request.Money, handler.maximumAmount)
	if err != nil {
		AbortErrorPage(context, http.StatusBadRequest, fmt.Sprintf("amount %q rejected (configured maximum %q): %s", request.Money, handler.maximumAmount, err.Error()))
		return
	}
	notifyURL, err := handler.notifyURLPolicy.Canonical(request.NotifyURL)
	if err != nil {
		AbortErrorPage(context, http.StatusBadRequest, fmt.Sprintf("notify_url %q rejected, allowlist is %v: %s", request.NotifyURL, handler.notifyURLPolicy.Allowed(), err.Error()))
		return
	}
	returnURL, err := handler.returnURLPolicy.Validate(context.Request.Context(), request.ReturnURL)
	if err != nil {
		AbortErrorPage(context, http.StatusBadRequest, fmt.Sprintf("return_url %q rejected: %s", request.ReturnURL, err.Error()))
		return
	}
	fingerprint := order.Fingerprint(request.PartnerID, request.PaymentType, request.MerchantOrder, request.Subject, amountText, notifyURL, returnURL.String())
	requestID, _ := context.Get(RequestIDHeader)

	token, err := newCashierToken()
	if err != nil {
		AbortErrorPage(context, http.StatusServiceUnavailable, "cashier token generation failed: "+err.Error())
		return
	}
	result, err := handler.store.CreatePaymentOrder(store.CreatePaymentOrderInput{
		ID: uuid.NewString(), OutTradeNo: request.MerchantOrder, GatewayTradeNo: "GW" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		RequestFingerprint: fingerprint, EpayPID: request.PartnerID, PaymentType: request.PaymentType, Subject: request.Subject,
		AmountText: amountText, AmountFen: amountFen, NotifyURL: notifyURL, ReturnURL: returnURL.String(),
		CashierTokenHash: order.HashCashierToken(token), ExpiresAt: time.Now().UTC().Add(order.OrderTTL), RequestID: requestID.(string),
	})
	if err != nil {
		AbortErrorPage(context, http.StatusServiceUnavailable, "persisting payment order failed: "+err.Error())
		return
	}
	if result.Conflict {
		AbortErrorPage(context, http.StatusConflict, "merchant order "+request.MerchantOrder+" already exists with different request parameters")
		return
	}
	if result.Existing {
		cookie, cookieErr := context.Request.Cookie(cashierCookieName(result.Order.ID))
		if cookieErr != nil || order.HashCashierToken(cookie.Value) != result.Order.CashierTokenHash {
			AbortErrorPage(context, http.StatusConflict, "replayed merchant order "+request.MerchantOrder+" without a matching cashier cookie")
			return
		}
		token = cookie.Value
	} else {
		context.SetCookie(cashierCookieName(result.Order.ID), token, int(order.OrderTTL.Seconds()), "/", "", true, true)
		if handler.nativeOrders != nil {
			if err := handler.nativeOrders.Create(context.Request.Context(), order.NativeOrderRecord{
				ID: result.Order.ID, OutTradeNo: result.Order.OutTradeNo, Subject: result.Order.Subject,
				AmountFen: result.Order.AmountFen, NotifyURL: handler.wechatNotifyURL, ExpiresAt: result.Order.ExpiresAt,
				Status: result.Order.Status, Version: result.Order.Version, CreatedAt: result.Order.CreatedAt,
			}); err != nil {
				AbortErrorPage(context, http.StatusServiceUnavailable, "creating wechat native order failed: "+err.Error())
				return
			}
		}
	}
	context.Redirect(http.StatusSeeOther, "/cashier/"+token)
}

func hasDuplicateField(form map[string][]string) bool {
	for _, values := range form {
		if len(values) != 1 {
			return true
		}
	}
	return false
}

func newCashierToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func cashierCookieName(orderID string) string {
	return cashierCookiePrefix + orderID
}
