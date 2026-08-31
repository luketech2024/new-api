package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

const (
	walletNotifyURL       = "https://api.example.com/api/user/epay/notify"
	subscriptionNotifyURL = "https://api.example.com/api/subscription/epay/notify"
)

func newWorkerFixtureWithNotifyURL(t *testing.T, notifyURL string) (*store.Store, store.NotificationTask, config.Config) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(db))
	repository := store.New(db)
	paymentOrder := store.PaymentOrder{
		ID: "delivery-order", OutTradeNo: "delivery-out-trade", GatewayTradeNo: "delivery-gateway", RequestFingerprint: "delivery-fingerprint",
		EpayPID: "10001", PaymentType: "wxpay", Subject: "Top up", AmountText: "1.00", AmountFen: 100,
		NotifyURL: notifyURL, CashierTokenHash: "delivery-token", Status: order.StatusPaidPendingNotify,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Version: 1,
	}
	require.NoError(t, repository.DB().Create(&paymentOrder).Error)
	payload, err := json.Marshal(store.NotificationPayload{
		PartnerID: paymentOrder.EpayPID, PaymentType: paymentOrder.PaymentType, MerchantOrderNo: paymentOrder.OutTradeNo,
		GatewayTradeNo: paymentOrder.GatewayTradeNo, Subject: paymentOrder.Subject, AmountText: paymentOrder.AmountText,
	})
	require.NoError(t, err)
	task := store.NotificationTask{ID: "delivery-task", OrderID: paymentOrder.ID, State: order.NotificationPending, PayloadSnapshot: string(payload), NextAttemptAt: time.Now().UTC().Add(-time.Second), Version: 1}
	require.NoError(t, repository.DB().Create(&task).Error)
	appConfig := config.Config{
		EpayPartnerID: "10001", EpayKey: "shared-key",
		NewAPINotifyURLs: []string{walletNotifyURL, subscriptionNotifyURL},
	}
	return repository, task, appConfig
}

func newWorkerFixture(t *testing.T) (*store.Store, store.NotificationTask, config.Config) {
	t.Helper()
	return newWorkerFixtureWithNotifyURL(t, walletNotifyURL)
}

func TestWorkerCompletesOnlyStrictEpaySuccessResponse(t *testing.T) {
	repository, task, appConfig := newWorkerFixture(t)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		values, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		assert.Equal(t, "delivery-gateway", values.Get("trade_no"))
		assert.Equal(t, "TRADE_SUCCESS", values.Get("trade_status"))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(" success\n")), Header: make(http.Header)}, nil
	})}
	worker, err := NewWorker(repository, appConfig, "worker-1", client)
	require.NoError(t, err)
	worker.now = func() time.Time { return time.Now().UTC() }

	require.NoError(t, worker.ProcessOne(context.Background()))
	var actualTask store.NotificationTask
	require.NoError(t, repository.DB().First(&actualTask, "id = ?", task.ID).Error)
	assert.Equal(t, order.NotificationSucceeded, actualTask.State)
	var actualOrder store.PaymentOrder
	require.NoError(t, repository.DB().First(&actualOrder, "id = ?", task.OrderID).Error)
	assert.Equal(t, order.StatusNotified, actualOrder.Status)
}

// A callback must reach the flow that created the order, so the destination comes from
// the order itself rather than from a single configured URL.
func TestWorkerDeliversToTheNotifyURLOfTheOrder(t *testing.T) {
	tests := []struct {
		name      string
		notifyURL string
	}{
		{name: "wallet top-up", notifyURL: walletNotifyURL},
		{name: "subscription purchase", notifyURL: subscriptionNotifyURL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, task, appConfig := newWorkerFixtureWithNotifyURL(t, test.notifyURL)
			var requested string
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requested = request.URL.String()
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success")), Header: make(http.Header)}, nil
			})}
			worker, err := NewWorker(repository, appConfig, "worker-1", client)
			require.NoError(t, err)

			require.NoError(t, worker.ProcessOne(context.Background()))
			assert.Equal(t, test.notifyURL, requested)
			var actualTask store.NotificationTask
			require.NoError(t, repository.DB().First(&actualTask, "id = ?", task.ID).Error)
			assert.Equal(t, order.NotificationSucceeded, actualTask.State)
		})
	}
}

func TestWorkerRefusesDeliveryToANotifyURLOutsideTheAllowlist(t *testing.T) {
	repository, task, appConfig := newWorkerFixtureWithNotifyURL(t, "https://api.example.com/api/removed/epay/notify")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("delivery must not be attempted")
	})}
	worker, err := NewWorker(repository, appConfig, "worker-1", client)
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOne(context.Background()))
	var actualTask store.NotificationTask
	require.NoError(t, repository.DB().First(&actualTask, "id = ?", task.ID).Error)
	assert.Equal(t, order.NotificationRetry, actualTask.State)
	require.NotNil(t, actualTask.LastError)
	assert.Contains(t, *actualTask.LastError, "no longer allowlisted")
}

func TestWorkerRetriesRejectedResponsesAndReclaimsExpiredLease(t *testing.T) {
	repository, task, appConfig := newWorkerFixture(t)
	now := time.Now().UTC()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("busy")), Header: make(http.Header)}, nil
	})}
	worker, err := NewWorker(repository, appConfig, "worker-1", client)
	require.NoError(t, err)
	worker.now = func() time.Time { return now }
	require.NoError(t, worker.ProcessOne(context.Background()))

	var retried store.NotificationTask
	require.NoError(t, repository.DB().First(&retried, "id = ?", task.ID).Error)
	assert.Equal(t, order.NotificationRetry, retried.State)
	require.NotNil(t, retried.LastHTTPStatus)
	assert.Equal(t, http.StatusTooManyRequests, *retried.LastHTTPStatus)
	assert.True(t, retried.NextAttemptAt.After(now))

	leaseOwner := "abandoned-worker"
	leaseUntil := now.Add(-time.Second)
	require.NoError(t, repository.DB().Model(&store.NotificationTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"state": order.NotificationProcessing, "lease_owner": leaseOwner, "lease_until": leaseUntil, "version": retried.Version + 1,
	}).Error)
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success")), Header: make(http.Header)}, nil
	})
	require.NoError(t, worker.ProcessOne(context.Background()))
	var reclaimed store.NotificationTask
	require.NoError(t, repository.DB().First(&reclaimed, "id = ?", task.ID).Error)
	assert.Equal(t, order.NotificationSucceeded, reclaimed.State)
}
