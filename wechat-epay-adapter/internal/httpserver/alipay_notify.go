package httpserver

import (
	"strings"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/observability"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/gin-gonic/gin"
)

type AlipayNotificationHandler struct {
	store    *store.Store
	verifier alipay.NotificationVerifier
	appID    string
	sellerID string
	metrics  *observability.Metrics
}

func NewAlipayNotificationHandler(database *store.Store, verifier alipay.NotificationVerifier, appConfig config.Config, metrics *observability.Metrics) *AlipayNotificationHandler {
	return &AlipayNotificationHandler{store: database, verifier: verifier, appID: appConfig.AlipayAppID, sellerID: appConfig.AlipaySellerID, metrics: metrics}
}

func (handler *AlipayNotificationHandler) Handle(context *gin.Context) {
	if err := context.Request.ParseForm(); err != nil {
		status, body := ResponseForAlipayNotification(AlipayNotificationInvalid)
		context.String(status, body)
		return
	}
	values := make(map[string]string, len(context.Request.PostForm))
	for name, items := range context.Request.PostForm {
		if len(items) > 0 {
			values[name] = items[0]
		}
	}
	if handler.verifier == nil {
		status, body := ResponseForAlipayNotification(AlipayNotificationInvalid)
		context.String(status, body)
		return
	}
	notice, err := handler.verifier.Verify(context.Request.Context(), values)
	if err != nil {
		if handler.metrics != nil {
			handler.metrics.ObserveChannel("alipay", "verify_fail")
		}
		status, body := ResponseForAlipayNotification(AlipayNotificationInvalid)
		context.String(status, body)
		return
	}
	if strings.TrimSpace(values["sign_type"]) != "" && strings.ToUpper(values["sign_type"]) != alipay.SignTypeRSA2 {
		status, body := ResponseForAlipayNotification(AlipayNotificationInvalid)
		context.String(status, body)
		return
	}
	result, err := handler.store.ConfirmAlipayPayment(store.ConfirmAlipayPaymentInput{Notice: notice, ExpectedAppID: handler.appID, ExpectedSellerID: handler.sellerID})
	if err != nil {
		status, body := ResponseForAlipayNotification(AlipayNotificationTemporary)
		context.String(status, body)
		return
	}
	outcome := AlipayNotificationPersisted
	if result.UnknownOrder {
		outcome = AlipayNotificationUnknownOrder
	}
	status, body := ResponseForAlipayNotification(outcome)
	context.String(status, body)
}