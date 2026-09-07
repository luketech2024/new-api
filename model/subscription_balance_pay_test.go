package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcSubscriptionBalanceQuota_UsesSettlementCurrency(t *testing.T) {
	prevPrice := operation_setting.Price
	operation_setting.Price = 7.3
	t.Cleanup(func() {
		operation_setting.Price = prevPrice
	})

	usdQuota, err := calcSubscriptionBalanceQuota(&SubscriptionPlan{
		PriceAmount: 20,
		Currency:    PlanCurrencyUSD,
	})
	require.NoError(t, err)
	assert.Equal(t, 10_000_000, usdQuota)

	cnyQuota, err := calcSubscriptionBalanceQuota(&SubscriptionPlan{
		PriceAmount: 20,
		Currency:    PlanCurrencyCNY,
	})
	require.NoError(t, err)
	assert.Equal(t, 1_369_864, cnyQuota)

	emptyCurrencyQuota, err := calcSubscriptionBalanceQuota(&SubscriptionPlan{
		PriceAmount: 20,
		Currency:    "",
	})
	require.NoError(t, err)
	assert.Equal(t, 10_000_000, emptyCurrencyQuota)
}

func TestPlanWeChatCNYYuan_MatchesEpayMoney(t *testing.T) {
	prevPrice := operation_setting.Price
	operation_setting.Price = 7.3
	t.Cleanup(func() {
		operation_setting.Price = prevPrice
	})

	usd, err := PlanWeChatCNYYuan(&SubscriptionPlan{PriceAmount: 20, Currency: PlanCurrencyUSD})
	require.NoError(t, err)
	gotUSD, _ := usd.Float64()
	assert.InDelta(t, 146.0, gotUSD, 0.001)

	cny, err := PlanWeChatCNYYuan(&SubscriptionPlan{PriceAmount: 20, Currency: PlanCurrencyCNY})
	require.NoError(t, err)
	gotCNY, _ := cny.Float64()
	assert.InDelta(t, 20.0, gotCNY, 0.001)
}

func TestCalcSubscriptionBalanceQuota_InvalidSalePriceRejectsCNY(t *testing.T) {
	prevPrice := operation_setting.Price
	operation_setting.Price = 0
	t.Cleanup(func() {
		operation_setting.Price = prevPrice
	})

	_, err := calcSubscriptionBalanceQuota(&SubscriptionPlan{
		PriceAmount: 20,
		Currency:    PlanCurrencyCNY,
	})
	require.Error(t, err)
	assert.Equal(t, errInvalidSalePrice.Error(), err.Error())

	_, err = PlanWeChatCNYYuan(&SubscriptionPlan{PriceAmount: 20, Currency: PlanCurrencyUSD})
	require.Error(t, err)
	assert.Equal(t, errInvalidSalePrice, err)
}

func TestValidateOptionValue_PriceAndExchangeRateMustBePositive(t *testing.T) {
	for _, key := range []string{"Price", "USDExchangeRate"} {
		assert.Error(t, validateOptionValue(key, "0"))
		assert.Error(t, validateOptionValue(key, "-1"))
		assert.Error(t, validateOptionValue(key, "NaN"))
		assert.Error(t, validateOptionValue(key, "inf"))
		require.NoError(t, validateOptionValue(key, "7.3"))
	}
	require.NoError(t, validateOptionValue("Price", "5"))
	require.NoError(t, validateOptionValue("USDExchangeRate", "8"))
}
