package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeSubscriptionQuote_USDAndCNYDisplayCombinations(t *testing.T) {
	const quotaPerUnit = 500000.0
	const salePrice = 7.3

	tests := []struct {
		name          string
		currency      string
		displayType   string
		dueAmount     float64
		dueCurrency   string
		wantNote      bool
		noteContains  string
		requiredQuota int
		weChatCNY     float64
	}{
		{
			name:          "USD_plan_display_USD",
			currency:      PlanCurrencyUSD,
			displayType:   operation_setting.QuotaDisplayTypeUSD,
			dueAmount:     20,
			dueCurrency:   PlanCurrencyUSD,
			requiredQuota: 10_000_000,
			weChatCNY:     146,
		},
		{
			name:          "USD_plan_display_CNY",
			currency:      PlanCurrencyUSD,
			displayType:   operation_setting.QuotaDisplayTypeCNY,
			dueAmount:     146,
			dueCurrency:   PlanCurrencyCNY,
			wantNote:      true,
			noteContains:  "$20.00",
			requiredQuota: 10_000_000,
			weChatCNY:     146,
		},
		{
			name:          "CNY_plan_display_CNY",
			currency:      PlanCurrencyCNY,
			displayType:   operation_setting.QuotaDisplayTypeCNY,
			dueAmount:     20,
			dueCurrency:   PlanCurrencyCNY,
			requiredQuota: 1_369_864,
			weChatCNY:     20,
		},
		{
			name:          "CNY_plan_display_USD",
			currency:      PlanCurrencyCNY,
			displayType:   operation_setting.QuotaDisplayTypeUSD,
			dueAmount:     2.74,
			dueCurrency:   PlanCurrencyUSD,
			wantNote:      true,
			noteContains:  "¥20.00",
			requiredQuota: 1_369_864,
			weChatCNY:     20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
				PriceAmount:  20,
				Currency:     tt.currency,
				SalePrice:    salePrice,
				QuotaPerUnit: quotaPerUnit,
				DisplayType:  tt.displayType,
			})
			require.True(t, quote.Balance.OK, quote.Balance.Error)
			assert.Equal(t, 20.0, quote.Settlement.Amount)
			assert.Equal(t, tt.currency, quote.Settlement.Currency)
			assert.InDelta(t, tt.dueAmount, quote.DueDisplay.Amount, 0.001)
			assert.Equal(t, tt.dueCurrency, quote.DueDisplay.Currency)
			assert.Equal(t, tt.requiredQuota, quote.Balance.RequiredQuota)
			if tt.wantNote {
				require.NotNil(t, quote.DueDisplay.NoteSettlement)
				assert.Equal(t, tt.noteContains, *quote.DueDisplay.NoteSettlement)
			} else {
				assert.Nil(t, quote.DueDisplay.NoteSettlement)
			}

			yuan, err := ComputeWeChatCNYYuan(20, tt.currency, salePrice)
			require.NoError(t, err)
			gotYuan, _ := yuan.Float64()
			assert.InDelta(t, tt.weChatCNY, gotYuan, 0.001)
		})
	}
}

func TestComputeSubscriptionQuote_DisplaySwitchDoesNotChangeRequiredQuotaOrWeChat(t *testing.T) {
	in := SubscriptionQuoteInput{
		PriceAmount:  20,
		Currency:     PlanCurrencyUSD,
		SalePrice:    7.3,
		QuotaPerUnit: 500000,
		DisplayType:  operation_setting.QuotaDisplayTypeUSD,
	}
	usdDisplay := ComputeSubscriptionQuote(in)
	in.DisplayType = operation_setting.QuotaDisplayTypeCNY
	cnyDisplay := ComputeSubscriptionQuote(in)

	require.True(t, usdDisplay.Balance.OK)
	require.True(t, cnyDisplay.Balance.OK)
	assert.Equal(t, usdDisplay.Balance.RequiredQuota, cnyDisplay.Balance.RequiredQuota)
	assert.Equal(t, 10_000_000, usdDisplay.Balance.RequiredQuota)

	a, err := ComputeWeChatCNYYuan(in.PriceAmount, in.Currency, in.SalePrice)
	require.NoError(t, err)
	b, err := ComputeWeChatCNYYuan(in.PriceAmount, in.Currency, in.SalePrice)
	require.NoError(t, err)
	assert.True(t, a.Equal(b))
}

func TestComputeSubscriptionQuote_EmptyCurrencyDefaultsToUSD(t *testing.T) {
	quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
		PriceAmount:  20,
		Currency:     "",
		SalePrice:    7.3,
		QuotaPerUnit: 500000,
		DisplayType:  operation_setting.QuotaDisplayTypeUSD,
	})
	require.True(t, quote.Balance.OK, quote.Balance.Error)
	assert.Equal(t, PlanCurrencyUSD, quote.Settlement.Currency)
	assert.Equal(t, 10_000_000, quote.Balance.RequiredQuota)
}

func TestComputeSubscriptionQuote_UnknownCurrency_Rejects(t *testing.T) {
	quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
		PriceAmount:  20,
		Currency:     "EUR",
		SalePrice:    7.3,
		QuotaPerUnit: 500000,
		DisplayType:  operation_setting.QuotaDisplayTypeUSD,
	})
	assert.False(t, quote.Balance.OK)
	assert.Equal(t, 0, quote.Balance.RequiredQuota)
	assert.Contains(t, quote.Balance.Error, "USD")
}

func TestComputeSubscriptionQuote_InvalidSalePrice_FailClosed(t *testing.T) {
	t.Run("CNY_quota_needs_sale_price", func(t *testing.T) {
		quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
			PriceAmount:  20,
			Currency:     PlanCurrencyCNY,
			SalePrice:    0,
			QuotaPerUnit: 500000,
			DisplayType:  operation_setting.QuotaDisplayTypeCNY,
		})
		assert.False(t, quote.Balance.OK)
		assert.Equal(t, 0, quote.Balance.RequiredQuota)
		assert.Equal(t, errInvalidSalePrice.Error(), quote.Balance.Error)
	})

	t.Run("USD_display_CNY_needs_sale_price", func(t *testing.T) {
		quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
			PriceAmount:  20,
			Currency:     PlanCurrencyUSD,
			SalePrice:    -1,
			QuotaPerUnit: 500000,
			DisplayType:  operation_setting.QuotaDisplayTypeCNY,
		})
		assert.False(t, quote.Balance.OK)
		assert.Equal(t, 10_000_000, quote.Balance.RequiredQuota)
		assert.Equal(t, errInvalidSalePrice.Error(), quote.Balance.Error)
	})

	t.Run("USD_same_display_still_computes_quota", func(t *testing.T) {
		quote := ComputeSubscriptionQuote(SubscriptionQuoteInput{
			PriceAmount:  20,
			Currency:     PlanCurrencyUSD,
			SalePrice:    0,
			QuotaPerUnit: 500000,
			DisplayType:  operation_setting.QuotaDisplayTypeUSD,
		})
		require.True(t, quote.Balance.OK, quote.Balance.Error)
		assert.Equal(t, 10_000_000, quote.Balance.RequiredQuota)
		_, err := ComputeWeChatCNYYuan(20, PlanCurrencyUSD, 0)
		require.Error(t, err)
		assert.Equal(t, errInvalidSalePrice, err)
	})
}

func TestComputeWeChatCNYYuan_DoesNotUseDisplayRate(t *testing.T) {
	yuan, err := ComputeWeChatCNYYuan(20, PlanCurrencyUSD, 7.3)
	require.NoError(t, err)
	got, _ := yuan.Float64()
	assert.InDelta(t, 146, got, 0.001)
}
