package order

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/epay"
)

const unknownCreateScanLimit = 100

type RecoveryRepository interface {
	ExpirePayableOrders(time.Time) (int64, error)
	FindCreateUnknownOrders(int) ([]NativeOrderRecord, error)
}

type RecoveryScheduler struct {
	repository RecoveryRepository
	native     *NativeOrderService
	precreate  *PrecreateService
	now        func() time.Time
}

func NewRecoveryScheduler(repository RecoveryRepository, native *NativeOrderService, precreate *PrecreateService) *RecoveryScheduler {
	return &RecoveryScheduler{repository: repository, native: native, precreate: precreate, now: func() time.Time { return time.Now().UTC() }}
}

func (scheduler *RecoveryScheduler) Run(ctx context.Context) {
	_ = scheduler.Process(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = scheduler.Process(ctx)
		}
	}
}

func (scheduler *RecoveryScheduler) Process(ctx context.Context) error {
	now := scheduler.now()
	if _, err := scheduler.repository.ExpirePayableOrders(now); err != nil {
		return err
	}
	records, err := scheduler.repository.FindCreateUnknownOrders(unknownCreateScanLimit)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.PaymentType == epay.PaymentTypeAlipay {
			if scheduler.precreate == nil {
				continue
			}
			if err := scheduler.precreate.RecoverUnknown(ctx, record); err != nil {
				return err
			}
			continue
		}
		if scheduler.native == nil {
			continue
		}
		if err := scheduler.native.RecoverUnknown(ctx, record); err != nil {
			return err
		}
	}
	return nil
}
