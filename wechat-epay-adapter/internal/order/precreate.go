package order

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/epay"
)

type PrecreateService struct {
	repository NativeOrderRepository
	alipay     alipay.Client
	now        func() time.Time
}

func NewPrecreateService(repository NativeOrderRepository, client alipay.Client) *PrecreateService {
	return &PrecreateService{repository: repository, alipay: client, now: func() time.Time { return time.Now().UTC() }}
}

func (service *PrecreateService) Create(ctx context.Context, record NativeOrderRecord) error {
	if record.Status != StatusCreating {
		return nil
	}
	result, err := service.alipay.Precreate(ctx, alipay.PrecreateRequest{
		MerchantOrderNo: record.OutTradeNo,
		TotalAmount:     record.AmountText,
		Subject:         record.Subject,
		TimeoutExpress:  "15m",
		NotifyURL:       record.NotifyURL,
	})
	if err != nil {
		return service.persistFailure(record, err)
	}
	if err := ValidateAlipayQRCode(result.QRCode); err != nil {
		return service.persistFailure(record, fmt.Errorf("%w: invalid qr_code", alipay.ErrRequestRejected))
	}
	code := result.QRCode
	_, err = service.repository.UpdateNativeOrder(record, NativeOrderUpdate{Status: StatusPayable, AlipayQRCode: &code})
	return err
}

func (service *PrecreateService) RecoverUnknown(ctx context.Context, record NativeOrderRecord) error {
	if record.Status != StatusCreateUnknown {
		return nil
	}
	if record.PaymentType != "" && record.PaymentType != epay.PaymentTypeAlipay {
		return nil
	}
	if !record.CreatedAt.IsZero() && !service.now().Before(record.CreatedAt.Add(UnknownCreateObservationWindow)) {
		_, err := service.repository.UpdateNativeOrder(record, NativeOrderUpdate{Status: StatusManualReview, ErrorCode: pointer("CREATE_UNKNOWN_TIMEOUT")})
		return err
	}
	query, err := service.alipay.Query(ctx, record.OutTradeNo)
	if err != nil {
		if errors.Is(err, alipay.ErrRequestRejected) {
			_, updateErr := service.repository.UpdateNativeOrder(record, NativeOrderUpdate{Status: StatusCreateFailed, ErrorCode: pointer("ALIPAY_QUERY_REJECTED")})
			return updateErr
		}
		return nil
	}
	if query.MerchantOrderNo != record.OutTradeNo || query.TotalAmount != record.AmountText {
		_, err := service.repository.UpdateNativeOrder(record, NativeOrderUpdate{Status: StatusManualReview, ErrorCode: pointer("ALIPAY_QUERY_MISMATCH")})
		return err
	}
	return nil
}

func (service *PrecreateService) persistFailure(record NativeOrderRecord, err error) error {
	update := NativeOrderUpdate{Status: StatusCreateFailed, ErrorCode: pointer("ALIPAY_CREATE_REJECTED")}
	if errors.Is(err, alipay.ErrResultUnknown) {
		update.Status = StatusCreateUnknown
		update.ErrorCode = pointer("ALIPAY_CREATE_UNKNOWN")
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	update.ErrorMessage = &message
	_, updateErr := service.repository.UpdateNativeOrder(record, update)
	return updateErr
}

func ValidateAlipayQRCode(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" {
		return errors.New("qr_code must be an absolute HTTPS Alipay URL")
	}
	if strings.EqualFold(parsed.Scheme, "weixin") {
		return errors.New("qr_code must not use a WeChat scheme")
	}
	if parsed.Scheme != "https" {
		return errors.New("qr_code must be an absolute HTTPS Alipay URL")
	}
	return nil
}
