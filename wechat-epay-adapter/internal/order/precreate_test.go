package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeAlipayClient struct {
	create func(context.Context, alipay.PrecreateRequest) (alipay.PrecreateOrder, error)
}

func (f fakeAlipayClient) Precreate(ctx context.Context, request alipay.PrecreateRequest) (alipay.PrecreateOrder, error) {
	return f.create(ctx, request)
}

func (f fakeAlipayClient) Query(context.Context, string) (alipay.OrderQuery, error) {
	return alipay.OrderQuery{}, nil
}

func TestPrecreatePersistsHTTPSQRCodeAndAmountText(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	repository := store.New(db)
	paymentOrder := store.PaymentOrder{
		ID: "ali-order", OutTradeNo: "merchant-ali-1", GatewayTradeNo: "gateway-ali-1", RequestFingerprint: "fingerprint",
		EpayPID: "10001", PaymentType: "alipay", Subject: "Top up", AmountText: "0.10", AmountFen: 10,
		NotifyURL: "https://pay.example.com/api/v1/alipay/notify", CashierTokenHash: "cashier-token", Status: order.StatusCreating,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Version: 1,
	}
	require.NoError(t, repository.DB().Create(&paymentOrder).Error)
	var captured alipay.PrecreateRequest
	service := order.NewPrecreateService(repository, fakeAlipayClient{create: func(_ context.Context, request alipay.PrecreateRequest) (alipay.PrecreateOrder, error) {
		captured = request
		return alipay.PrecreateOrder{QRCode: "https://qr.alipay.com/bax10"}, nil
	}})
	require.NoError(t, service.Create(context.Background(), order.NativeOrderRecord{
		ID: paymentOrder.ID, OutTradeNo: paymentOrder.OutTradeNo, Subject: paymentOrder.Subject,
		AmountFen: paymentOrder.AmountFen, AmountText: paymentOrder.AmountText, PaymentType: paymentOrder.PaymentType,
		NotifyURL: paymentOrder.NotifyURL, ExpiresAt: paymentOrder.ExpiresAt, Status: paymentOrder.Status, Version: paymentOrder.Version,
	}))
	assert.Equal(t, "0.10", captured.TotalAmount)
	var actual store.PaymentOrder
	require.NoError(t, repository.DB().First(&actual, "id = ?", paymentOrder.ID).Error)
	assert.Equal(t, order.StatusPayable, actual.Status)
	require.NotNil(t, actual.AlipayQrCode)
	assert.Equal(t, "https://qr.alipay.com/bax10", *actual.AlipayQrCode)
	assert.Nil(t, actual.WechatCodeURL)
}

func TestPrecreateRejectsWeixinScheme(t *testing.T) {
	assert.Error(t, order.ValidateAlipayQRCode("weixin://wxpay/bizpayurl?pr=test"))
	assert.NoError(t, order.ValidateAlipayQRCode("https://qr.alipay.com/bax0001"))
}

func TestPrecreateClassifiesUnknownResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	repository := store.New(db)
	paymentOrder := store.PaymentOrder{
		ID: "ali-unknown", OutTradeNo: "merchant-ali-unknown", GatewayTradeNo: "gateway-ali-unknown", RequestFingerprint: "fingerprint",
		EpayPID: "10001", PaymentType: "alipay", Subject: "Top up", AmountText: "1.01", AmountFen: 101,
		NotifyURL: "https://pay.example.com/api/v1/alipay/notify", CashierTokenHash: "cashier-token-2", Status: order.StatusCreating,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Version: 1,
	}
	require.NoError(t, repository.DB().Create(&paymentOrder).Error)
	service := order.NewPrecreateService(repository, fakeAlipayClient{create: func(context.Context, alipay.PrecreateRequest) (alipay.PrecreateOrder, error) {
		return alipay.PrecreateOrder{}, alipay.ErrResultUnknown
	}})
	require.NoError(t, service.Create(context.Background(), order.NativeOrderRecord{
		ID: paymentOrder.ID, OutTradeNo: paymentOrder.OutTradeNo, Subject: paymentOrder.Subject,
		AmountFen: paymentOrder.AmountFen, AmountText: paymentOrder.AmountText, PaymentType: paymentOrder.PaymentType,
		NotifyURL: paymentOrder.NotifyURL, Status: paymentOrder.Status, Version: paymentOrder.Version,
	}))
	var actual store.PaymentOrder
	require.NoError(t, repository.DB().First(&actual, "id = ?", paymentOrder.ID).Error)
	assert.Equal(t, order.StatusCreateUnknown, actual.Status)
}
