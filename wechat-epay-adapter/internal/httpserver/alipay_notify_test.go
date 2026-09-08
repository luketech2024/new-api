package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeAlipayVerifier struct {
	notice alipay.PaymentNotice
	err    error
}

func (f fakeAlipayVerifier) Verify(context.Context, map[string]string) (alipay.PaymentNotice, error) {
	return f.notice, f.err
}

func TestAlipayNotificationPersistsVerifiedPaymentWithSuccessBody(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	paymentOrder := store.PaymentOrder{
		ID: "ali-notify", OutTradeNo: "ali-out-trade", GatewayTradeNo: "ali-gateway", RequestFingerprint: "fp",
		EpayPID: "10001", PaymentType: "alipay", Subject: "Top up", AmountText: "1.00", AmountFen: 100,
		NotifyURL: "https://api.example.com/api/user/epay/notify", CashierTokenHash: "token", Status: order.StatusPayable,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Version: 1,
	}
	require.NoError(t, database.DB().Create(&paymentOrder).Error)
	router := gin.New()
	notice := alipay.PaymentNotice{
		NotifyID: "nid", AppID: "app", MerchantOrderNo: paymentOrder.OutTradeNo, TradeNo: "20260907001",
		TradeStatus: alipay.TradeSuccess, TotalAmount: "1.00",
	}
	router.POST(RouteAlipayNotification, NewAlipayNotificationHandler(database, fakeAlipayVerifier{notice: notice}, config.Config{AlipayAppID: "app"}, nil).Handle)
	form := url.Values{"sign_type": {"RSA2"}, "out_trade_no": {paymentOrder.OutTradeNo}}
	request := httptest.NewRequest(http.MethodPost, RouteAlipayNotification, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "success", response.Body.String())
	var actual store.PaymentOrder
	require.NoError(t, database.DB().First(&actual, "id = ?", paymentOrder.ID).Error)
	assert.Equal(t, order.StatusPaidPendingNotify, actual.Status)
}

func TestAlipayNotificationRejectsInvalidSignatureWithoutSettlement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	paymentOrder := store.PaymentOrder{
		ID: "ali-bad", OutTradeNo: "ali-bad-trade", GatewayTradeNo: "ali-gateway-2", RequestFingerprint: "fp",
		EpayPID: "10001", PaymentType: "alipay", Subject: "Top up", AmountText: "1.00", AmountFen: 100,
		NotifyURL: "https://api.example.com/api/user/epay/notify", CashierTokenHash: "token-2", Status: order.StatusPayable,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Version: 1,
	}
	require.NoError(t, database.DB().Create(&paymentOrder).Error)
	router := gin.New()
	router.POST(RouteAlipayNotification, NewAlipayNotificationHandler(database, fakeAlipayVerifier{err: alipay.ErrInvalidNotice}, config.Config{AlipayAppID: "app"}, nil).Handle)
	form := url.Values{"sign_type": {"RSA2"}, "out_trade_no": {paymentOrder.OutTradeNo}}
	request := httptest.NewRequest(http.MethodPost, RouteAlipayNotification, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.NotEqual(t, "success", strings.TrimSpace(response.Body.String()))
	var actual store.PaymentOrder
	require.NoError(t, database.DB().First(&actual, "id = ?", paymentOrder.ID).Error)
	assert.Equal(t, order.StatusPayable, actual.Status)
}

func TestAlipayNotificationUnknownOrderStillReturnsSuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	router := gin.New()
	notice := alipay.PaymentNotice{NotifyID: "missing", AppID: "app", MerchantOrderNo: "missing-order", TradeNo: "t", TradeStatus: alipay.TradeSuccess, TotalAmount: "1.00"}
	router.POST(RouteAlipayNotification, NewAlipayNotificationHandler(database, fakeAlipayVerifier{notice: notice}, config.Config{AlipayAppID: "app"}, nil).Handle)
	form := url.Values{"sign_type": {"RSA2"}}
	request := httptest.NewRequest(http.MethodPost, RouteAlipayNotification, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "success", response.Body.String())
	var taskCount int64
	require.NoError(t, database.DB().Model(&store.NotificationTask{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}
