package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/epay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
)

const (
	notificationLeaseDuration   = time.Minute
	maximumNotificationAttempts = 20
)

type Worker struct {
	store           *store.Store
	client          *http.Client
	notifyURLPolicy *order.NotifyURLPolicy
	partnerID       string
	key             string
	workerID        string
	now             func() time.Time
}

func NewWorker(database *store.Store, appConfig config.Config, workerID string, client *http.Client) (*Worker, error) {
	notifyURLPolicy, err := order.NewNotifyURLPolicy(appConfig.NewAPINotifyURLs)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = NewHTTPClient(ValidateAllowlistedDestination(notifyURLPolicy))
	}
	return &Worker{
		store: database, client: client, notifyURLPolicy: notifyURLPolicy, partnerID: appConfig.EpayPartnerID,
		key: appConfig.EpayKey, workerID: workerID, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run polls durable tasks until the application context is canceled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := w.ProcessOne(ctx); err != nil {
			// Persistent task state records the failure; the next scan can recover it.
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOne(ctx context.Context) error {
	now := w.now()
	task, found, err := w.store.ClaimNotificationTask(now, w.workerID, notificationLeaseDuration)
	if err != nil || !found {
		return err
	}
	payload := store.NotificationPayload{}
	if err := json.Unmarshal([]byte(task.PayloadSnapshot), &payload); err != nil {
		return w.fail(task, nil, "invalid notification payload")
	}
	params := map[string]string{
		"pid": payload.PartnerID, "type": payload.PaymentType, "out_trade_no": payload.MerchantOrderNo,
		"trade_no": payload.GatewayTradeNo, "name": payload.Subject, "money": payload.AmountText,
		"trade_status": epay.TradeStatusSuccess, "sign_type": epay.SignTypeMD5,
	}
	if payload.PartnerID != w.partnerID || !epay.AllowedPaymentType(payload.PaymentType) {
		return w.fail(task, nil, "notification payload does not match configured merchant")
	}
	storedDestination, err := w.store.NotifyDestination(task.OrderID)
	if err != nil {
		return w.fail(task, nil, "cannot load the notification destination of the order")
	}
	destination, err := w.notifyURLPolicy.Canonical(storedDestination)
	if err != nil {
		return w.fail(task, nil, "order notification destination is no longer allowlisted")
	}
	params["sign"] = epay.Sign(params, w.key)
	requestContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, destination, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return w.fail(task, nil, "cannot construct notification request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := w.client.Do(request)
	if err != nil {
		return w.fail(task, nil, "notification delivery failed")
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if readErr != nil {
		return w.fail(task, &response.StatusCode, "notification response could not be read")
	}
	if !epay.CallbackAccepted(response.StatusCode, body) {
		return w.fail(task, &response.StatusCode, fmt.Sprintf("notification response rejected with status %d", response.StatusCode))
	}
	return w.store.CompleteNotificationTask(task, w.now())
}

func (w *Worker) fail(task store.ClaimedNotificationTask, statusCode *int, failure string) error {
	now := w.now()
	dead := task.AttemptCount >= maximumNotificationAttempts
	nextAttempt := now.Add(notificationBackoff(task.AttemptCount))
	if dead {
		nextAttempt = now
	}
	if err := w.store.RescheduleNotificationTask(task, now, nextAttempt, failure, statusCode, dead); err != nil {
		return err
	}
	return nil
}

func notificationBackoff(attempt int) time.Duration {
	delays := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour}
	if attempt < 1 {
		return delays[0]
	}
	if attempt <= len(delays) {
		return delays[attempt-1]
	}
	return 6 * time.Hour
}
