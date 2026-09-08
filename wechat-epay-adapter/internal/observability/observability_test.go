package observability

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyLogWriterRotatesByDate(t *testing.T) {
	dir := t.TempDir()
	firstDay := time.Date(2026, time.August, 18, 23, 59, 59, 0, time.Local)
	writer := &dailyLogWriter{dir: dir, now: func() time.Time { return firstDay }}
	t.Cleanup(func() {
		if writer.file != nil {
			require.NoError(t, writer.file.Close())
		}
	})

	_, err := writer.Write([]byte("first\n"))
	require.NoError(t, err)
	writer.now = func() time.Time { return firstDay.AddDate(0, 0, 1) }
	_, err = writer.Write([]byte("second\n"))
	require.NoError(t, err)

	firstData, err := os.ReadFile(filepath.Join(dir, "20260818", "wechat-epay.log"))
	require.NoError(t, err)
	secondData, err := os.ReadFile(filepath.Join(dir, "20260819", "wechat-epay.log"))
	require.NoError(t, err)
	assert.Equal(t, "first\n", string(firstData))
	assert.Equal(t, "second\n", string(secondData))
}

func TestDailyLogWriterAddsSequenceAfterLogCountLimit(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.Local)
	writer := &dailyLogWriter{dir: dir, now: func() time.Time { return now }}
	t.Cleanup(func() {
		if writer.file != nil {
			require.NoError(t, writer.file.Close())
		}
	})

	_, err := writer.Write([]byte("first\n"))
	require.NoError(t, err)
	writer.count = maxLogCount
	_, err = writer.Write([]byte("next\n"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "20260818", "wechat-epay-1.log"))
	require.NoError(t, err)
	assert.Equal(t, "next\n", string(data))
}

func TestMetricsExposeOnlyBoundedLabelsAndStateCounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	database := store.New(db)
	require.NoError(t, database.DB().Create(&store.PaymentOrder{
		ID: "metrics-order", OutTradeNo: "sensitive-order-number", GatewayTradeNo: "gateway", RequestFingerprint: "fingerprint",
		EpayPID: "10001", PaymentType: "wxpay", Subject: "Top up", AmountText: "1.00", AmountFen: 100,
		NotifyURL: "https://pay.example.com/notify", CashierTokenHash: "token-hash", Status: order.StatusPayable,
		ExpiresAt: time.Now().UTC().Add(time.Hour), Version: 1,
	}).Error)

	metrics := NewMetrics(database)
	metrics.ObserveRequest("/api/v1/cashier/:access_token/status", http.MethodGet, http.StatusOK, 20*time.Millisecond)
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `payment_order_state{state="PAYABLE"} 1`)
	assert.Contains(t, response.Body.String(), `route="/api/v1/cashier/:access_token/status",method="GET",status="200"`)
	assert.NotContains(t, response.Body.String(), "sensitive-order-number")
	assert.NotContains(t, response.Body.String(), "https://pay.example.com/notify")
}

func TestMetricsExposeChannelEventsWithoutOrderIdentifiers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	metrics := NewMetrics(store.New(db))
	metrics.ObserveChannel("alipay", "verify_fail")
	metrics.ObserveChannel("wxpay", "paid")
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `payment_channel_events{channel="alipay",event="verify_fail"} 1`)
	assert.Contains(t, response.Body.String(), `payment_channel_events{channel="wxpay",event="paid"} 1`)
	assert.NotContains(t, response.Body.String(), "USR1NO")
	assert.NotContains(t, response.Body.String(), "private_key")
}
