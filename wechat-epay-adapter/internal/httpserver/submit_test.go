package httpserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/epay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/wechat"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	walletNotifyURL       = "https://api.example.com/api/user/epay/notify"
	subscriptionNotifyURL = "https://api.example.com/api/subscription/epay/notify"
)

func newSubmitConfig() config.Config {
	return config.Config{
		EpayPartnerID: "10001", EpayKey: "shared-secret",
		NewAPINotifyURLs:   []string{walletNotifyURL, subscriptionNotifyURL},
		MaxOrderAmountYuan: "5000.00",
	}
}

func newSubmitPolicies(t *testing.T) (*order.ReturnURLPolicy, *order.NotifyURLPolicy) {
	t.Helper()
	resolver := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	returnPolicy, err := order.NewReturnURLPolicy("https://app.example.com/console/", resolver)
	require.NoError(t, err)
	notifyPolicy, err := order.NewNotifyURLPolicy(newSubmitConfig().NewAPINotifyURLs)
	require.NoError(t, err)
	return returnPolicy, notifyPolicy
}

func newSubmitRouter(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	returnPolicy, notifyPolicy := newSubmitPolicies(t)
	router := gin.New()
	require.NoError(t, applySecurityMiddleware(router, SecurityOptions{}))
	router.POST(RouteSubmit, NewSubmitHandler(database, newSubmitConfig(), returnPolicy, notifyPolicy).Handle)
	return router, database
}

type submitWechatClient struct {
	request wechat.NativeOrderRequest
}

func (client *submitWechatClient) CreateNativeOrder(_ context.Context, request wechat.NativeOrderRequest) (wechat.NativeOrder, error) {
	client.request = request
	return wechat.NativeOrder{CodeURL: "weixin://wxpay/bizpayurl?pr=test"}, nil
}

func (client *submitWechatClient) QueryOrder(context.Context, string) (wechat.OrderQuery, error) {
	return wechat.OrderQuery{}, nil
}
func signedSubmitForm(subject, outTradeNo, notifyURL string) url.Values {
	params := map[string]string{
		"pid": "10001", "type": "wxpay", "out_trade_no": outTradeNo, "notify_url": notifyURL,
		"return_url": "https://app.example.com/console/billing", "name": subject, "money": "1.00", "device": "pc", "sign_type": "MD5",
	}
	params["sign"] = epay.Sign(params, "shared-secret")
	form := url.Values{}
	for name, value := range params {
		form.Set(name, value)
	}
	return form
}

func submitRequest(form url.Values, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(http.MethodPost, RouteSubmit, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func TestSubmitCreatesOrderAndCookieBasedIdempotentRedirect(t *testing.T) {
	router, database := newSubmitRouter(t)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, submitRequest(signedSubmitForm("TUC100", "USR1NO123", walletNotifyURL), nil))
	require.Equal(t, http.StatusSeeOther, first.Code)
	firstLocation := first.Header().Get("Location")
	assert.Regexp(t, `^/cashier/[A-Za-z0-9_-]{43}$`, firstLocation)
	cookies := first.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].HttpOnly)
	assert.True(t, cookies[0].Secure)

	repeated := httptest.NewRecorder()
	router.ServeHTTP(repeated, submitRequest(signedSubmitForm("TUC100", "USR1NO123", walletNotifyURL), cookies[0]))
	assert.Equal(t, http.StatusSeeOther, repeated.Code)
	assert.Equal(t, firstLocation, repeated.Header().Get("Location"))

	var count int64
	require.NoError(t, database.DB().Model(&store.PaymentOrder{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestSubmitUsesWechatCallbackForNativeOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	returnPolicy, notifyPolicy := newSubmitPolicies(t)
	client := &submitWechatClient{}
	nativeOrders := order.NewNativeOrderService(database, client)
	appConfig := newSubmitConfig()
	appConfig.WechatNotifyURL = "https://pay.example.com/api/v1/wechat/notify"
	router := gin.New()
	require.NoError(t, applySecurityMiddleware(router, SecurityOptions{}))
	router.POST(RouteSubmit, NewSubmitHandler(database, appConfig, returnPolicy, notifyPolicy, nativeOrders).Handle)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, submitRequest(signedSubmitForm("TUC100", "USR1NO123", walletNotifyURL), nil))
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, appConfig.WechatNotifyURL, client.request.NotifyURL)
	assert.NotContains(t, appConfig.NewAPINotifyURLs, client.request.NotifyURL)
}

// Wallet top-up and subscription purchase reach new-api on different callback paths,
// so intake must accept both and persist the path the order was submitted with.
func TestSubmitAcceptsEveryAllowlistedNotifyURLAndPersistsIt(t *testing.T) {
	tests := []struct {
		name       string
		outTradeNo string
		notifyURL  string
	}{
		{name: "wallet top-up", outTradeNo: "USR1NO123", notifyURL: walletNotifyURL},
		{name: "subscription purchase", outTradeNo: "SUBUSR1NO123", notifyURL: subscriptionNotifyURL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, database := newSubmitRouter(t)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, submitRequest(signedSubmitForm("TUC100", test.outTradeNo, test.notifyURL), nil))

			require.Equal(t, http.StatusSeeOther, response.Code)
			var created store.PaymentOrder
			require.NoError(t, database.DB().First(&created, "out_trade_no = ?", test.outTradeNo).Error)
			assert.Equal(t, test.notifyURL, created.NotifyURL)
		})
	}
}

func TestSubmitRejectsNotifyURLOutsideAllowlist(t *testing.T) {
	router, _ := newSubmitRouter(t)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, submitRequest(signedSubmitForm("TUC100", "USR1NO123", "https://api.example.com/api/other/epay/notify"), nil))
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSubmitRejectsConflictingOrderAndAuditsIt(t *testing.T) {
	router, database := newSubmitRouter(t)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, submitRequest(signedSubmitForm("TUC100", "USR1NO123", walletNotifyURL), nil))
	require.Equal(t, http.StatusSeeOther, first.Code)

	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, submitRequest(signedSubmitForm("TUC200", "USR1NO123", walletNotifyURL), nil))
	assert.Equal(t, http.StatusConflict, conflict.Code)
	assert.NotContains(t, conflict.Body.String(), "shared-secret")

	var audits int64
	require.NoError(t, database.DB().Model(&store.PaymentAuditEvent{}).Where("event_type = ?", "ORDER_CONFLICT").Count(&audits).Error)
	assert.Equal(t, int64(1), audits)
}
