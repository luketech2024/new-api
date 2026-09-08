package alipay

import (
	"context"
	"errors"
)

const (
	LockedSDKModule  = "github.com/smartwalle/alipay/v3"
	LockedSDKVersion = "v3.2.31"
	MethodPrecreate  = "alipay.trade.precreate"
	SignTypeRSA2     = "RSA2"
	TradeSuccess     = "TRADE_SUCCESS"
	TradeFinished    = "TRADE_FINISHED"
)

var (
	ErrRequestRejected = errors.New("alipay request rejected")
	ErrResultUnknown   = errors.New("alipay result unknown")
	ErrInvalidNotice   = errors.New("alipay notification invalid")
)

// PrecreateRequest is the domain payload for alipay.trade.precreate.
type PrecreateRequest struct {
	MerchantOrderNo string
	TotalAmount     string
	Subject         string
	TimeoutExpress  string
	NotifyURL       string
}

type PrecreateOrder struct {
	QRCode string
}

type OrderQuery struct {
	MerchantOrderNo string
	TradeNo         string
	TradeStatus     string
	TotalAmount     string
}

// PaymentNotice is the verified asynchronous notification after RSA2 checks.
type PaymentNotice struct {
	NotifyID        string
	AppID           string
	SellerID        string
	MerchantOrderNo string
	TradeNo         string
	TradeStatus     string
	TotalAmount     string
}

func PaidTradeStatus(status string) bool {
	return status == TradeSuccess || status == TradeFinished
}

// Client isolates the official SDK request and response types from domain code.
type Client interface {
	Precreate(context.Context, PrecreateRequest) (PrecreateOrder, error)
	Query(context.Context, string) (OrderQuery, error)
}

// NotificationVerifier validates RSA2 after dropping sign and sign_type.
type NotificationVerifier interface {
	Verify(context.Context, map[string]string) (PaymentNotice, error)
}
