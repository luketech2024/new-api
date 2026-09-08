package alipay

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractClient struct{}

func (contractClient) Precreate(context.Context, PrecreateRequest) (PrecreateOrder, error) {
	return PrecreateOrder{}, nil
}

func (contractClient) Query(context.Context, string) (OrderQuery, error) {
	return OrderQuery{}, nil
}

func TestClientContractUsesDomainTypes(t *testing.T) {
	var client Client = contractClient{}
	assert.NotNil(t, client)
	assert.Equal(t, "github.com/smartwalle/alipay/v3", LockedSDKModule)
	assert.Equal(t, "v3.2.31", LockedSDKVersion)
	assert.Equal(t, "alipay.trade.precreate", MethodPrecreate)
	assert.Equal(t, "RSA2", SignTypeRSA2)
}

func TestPaidTradeStatus_SuccessOrFinished_AllowsSettlement(t *testing.T) {
	assert.True(t, PaidTradeStatus(TradeSuccess))
	assert.True(t, PaidTradeStatus(TradeFinished))
	assert.False(t, PaidTradeStatus("WAIT_BUYER_PAY"))
	assert.False(t, PaidTradeStatus("TRADE_CLOSED"))
	assert.False(t, PaidTradeStatus(""))
}

func TestPrecreateRequestKeepsAmountAsDecimalText(t *testing.T) {
	request := PrecreateRequest{
		MerchantOrderNo: "USR1NO123",
		TotalAmount:     "0.01",
		Subject:         "TUC100",
	}
	require.Equal(t, "0.01", request.TotalAmount)
	assert.NotEqual(t, 0.01, request.TotalAmount)
}
