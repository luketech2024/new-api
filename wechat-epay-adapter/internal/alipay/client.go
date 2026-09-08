package alipay

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	sdk "github.com/smartwalle/alipay/v3"
)

type SDKClient struct {
	client *sdk.Client
}

func NewSDKClient(appConfig config.Config) (*SDKClient, error) {
	privateKey, err := os.ReadFile(appConfig.AlipayPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Alipay private key: %w", err)
	}
	publicKey, err := os.ReadFile(appConfig.AlipayPublicKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Alipay public key: %w", err)
	}
	production := isProductionGateway(appConfig.AlipayGateway)
	opts := []sdk.OptionFunc{sdk.WithHTTPClient(&http.Client{Timeout: 15 * time.Second})}
	if production {
		opts = append(opts, sdk.WithProductionGateway(appConfig.AlipayGateway))
	} else {
		opts = append(opts, sdk.WithSandboxGateway(appConfig.AlipayGateway))
	}
	client, err := sdk.New(appConfig.AlipayAppID, string(privateKey), production, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Alipay client: %w", err)
	}
	if err := client.LoadAliPayPublicKey(string(publicKey)); err != nil {
		return nil, fmt.Errorf("load Alipay public key material: %w", err)
	}
	return &SDKClient{client: client}, nil
}

func isProductionGateway(gateway string) bool {
	host := strings.ToLower(gateway)
	return !strings.Contains(host, "alipaydev") && !strings.Contains(host, "sandbox")
}

func (c *SDKClient) Precreate(ctx context.Context, request PrecreateRequest) (PrecreateOrder, error) {
	result, err := c.client.TradePreCreate(ctx, sdk.TradePreCreate{
		Trade: sdk.Trade{
			NotifyURL:      request.NotifyURL,
			Subject:        request.Subject,
			OutTradeNo:     request.MerchantOrderNo,
			TotalAmount:    request.TotalAmount,
			TimeoutExpress: request.TimeoutExpress,
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return PrecreateOrder{}, fmt.Errorf("%w: %v", ErrResultUnknown, err)
		}
		return PrecreateOrder{}, fmt.Errorf("%w: %v", ErrRequestRejected, err)
	}
	if result == nil || !result.IsSuccess() || strings.TrimSpace(result.QRCode) == "" {
		return PrecreateOrder{}, fmt.Errorf("%w: precreate did not return qr_code", ErrRequestRejected)
	}
	return PrecreateOrder{QRCode: result.QRCode}, nil
}

func (c *SDKClient) Query(ctx context.Context, merchantOrderNo string) (OrderQuery, error) {
	result, err := c.client.TradeQuery(ctx, sdk.TradeQuery{OutTradeNo: merchantOrderNo})
	if err != nil {
		if ctx.Err() != nil {
			return OrderQuery{}, fmt.Errorf("%w: %v", ErrResultUnknown, err)
		}
		return OrderQuery{}, fmt.Errorf("%w: %v", ErrRequestRejected, err)
	}
	if result == nil || !result.IsSuccess() {
		return OrderQuery{}, fmt.Errorf("%w: trade query rejected", ErrRequestRejected)
	}
	return OrderQuery{
		MerchantOrderNo: result.OutTradeNo,
		TradeNo:         result.TradeNo,
		TradeStatus:     string(result.TradeStatus),
		TotalAmount:     result.TotalAmount,
	}, nil
}

func (c *SDKClient) Verify(_ context.Context, values map[string]string) (PaymentNotice, error) {
	form := url.Values{}
	for name, value := range values {
		form.Set(name, value)
	}
	notice, err := c.client.DecodeNotification(context.Background(), form)
	if err != nil {
		return PaymentNotice{}, fmt.Errorf("%w: %v", ErrInvalidNotice, err)
	}
	return PaymentNotice{
		NotifyID:        notice.NotifyId,
		AppID:           notice.AppId,
		SellerID:        notice.SellerId,
		MerchantOrderNo: notice.OutTradeNo,
		TradeNo:         notice.TradeNo,
		TradeStatus:     string(notice.TradeStatus),
		TotalAmount:     notice.TotalAmount,
	}, nil
}
