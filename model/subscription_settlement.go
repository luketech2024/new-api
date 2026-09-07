package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

const (
	PlanCurrencyUSD = "USD"
	PlanCurrencyCNY = "CNY"
)

var (
	errInvalidSalePrice        = errors.New("充值售价未配置或无效")
	errInvalidQuotaPerUnit     = errors.New("额度单位配置错误")
	errUnsupportedPlanCurrency = errors.New("套餐结算币种仅支持 USD 或 CNY")
)

// SubscriptionQuoteInput is the explicit input for subscription settlement.
// DisplayType only affects DueDisplay; it must not change RequiredQuota or WeChat CNY.
type SubscriptionQuoteInput struct {
	PriceAmount  float64
	Currency     string
	SalePrice    float64
	QuotaPerUnit float64
	DisplayType  string
}

type SubscriptionSettlement struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type SubscriptionDueDisplay struct {
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	NoteSettlement *string `json:"note_settlement"`
}

type SubscriptionBalanceNeed struct {
	RequiredQuota int    `json:"required_quota"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
}

type SubscriptionQuote struct {
	Settlement SubscriptionSettlement  `json:"settlement"`
	DueDisplay SubscriptionDueDisplay  `json:"due_display"`
	Balance    SubscriptionBalanceNeed `json:"balance"`
}

func NormalizePlanCurrency(raw string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if c == "" {
		return PlanCurrencyUSD, nil
	}
	if c == PlanCurrencyUSD || c == PlanCurrencyCNY {
		return c, nil
	}
	return "", errUnsupportedPlanCurrency
}

func QuoteFromPlan(plan *SubscriptionPlan) SubscriptionQuote {
	if plan == nil {
		return SubscriptionQuote{
			Balance: SubscriptionBalanceNeed{OK: false, Error: "套餐不存在"},
		}
	}
	return ComputeSubscriptionQuote(SubscriptionQuoteInput{
		PriceAmount:  plan.PriceAmount,
		Currency:     plan.Currency,
		SalePrice:    operation_setting.Price,
		QuotaPerUnit: common.QuotaPerUnit,
		DisplayType:  operation_setting.GetQuotaDisplayType(),
	})
}

func RequiredQuotaFromPlan(plan *SubscriptionPlan) (int, error) {
	if plan == nil {
		return 0, errors.New("套餐不存在")
	}
	currency, err := NormalizePlanCurrency(plan.Currency)
	if err != nil {
		return 0, err
	}
	return computeRequiredQuota(plan.PriceAmount, currency, operation_setting.Price, common.QuotaPerUnit)
}

func PlanWeChatCNYYuan(plan *SubscriptionPlan) (decimal.Decimal, error) {
	if plan == nil {
		return decimal.Zero, errors.New("套餐不存在")
	}
	return ComputeWeChatCNYYuan(plan.PriceAmount, plan.Currency, operation_setting.Price)
}

func ComputeSubscriptionQuote(in SubscriptionQuoteInput) SubscriptionQuote {
	currency, err := NormalizePlanCurrency(in.Currency)
	if err != nil {
		return SubscriptionQuote{
			Settlement: SubscriptionSettlement{Amount: in.PriceAmount, Currency: strings.TrimSpace(in.Currency)},
			Balance:    SubscriptionBalanceNeed{OK: false, Error: err.Error()},
		}
	}

	quote := SubscriptionQuote{
		Settlement: SubscriptionSettlement{Amount: in.PriceAmount, Currency: currency},
	}

	quota, quotaErr := computeRequiredQuota(in.PriceAmount, currency, in.SalePrice, in.QuotaPerUnit)
	due, dueErr := computeDueDisplay(in.PriceAmount, currency, in.SalePrice, in.DisplayType)

	quote.Balance.RequiredQuota = quota
	quote.DueDisplay = due
	switch {
	case quotaErr != nil:
		quote.Balance.Error = quotaErr.Error()
	case dueErr != nil:
		quote.Balance.Error = dueErr.Error()
	default:
		quote.Balance.OK = true
	}
	return quote
}

func ComputeWeChatCNYYuan(priceAmount float64, currency string, salePrice float64) (decimal.Decimal, error) {
	normalized, err := NormalizePlanCurrency(currency)
	if err != nil {
		return decimal.Zero, err
	}
	price := decimal.NewFromFloat(priceAmount)
	if price.LessThan(decimal.Zero) {
		return decimal.Zero, errors.New("价格不能为负数")
	}
	if normalized == PlanCurrencyCNY {
		return price.Round(2), nil
	}
	if !salePricePositive(salePrice) {
		return decimal.Zero, errInvalidSalePrice
	}
	return price.Mul(decimal.NewFromFloat(salePrice)).Round(2), nil
}

func computeRequiredQuota(priceAmount float64, currency string, salePrice float64, quotaPerUnit float64) (int, error) {
	if priceAmount <= 0 {
		return 0, nil
	}
	if quotaPerUnit <= 0 {
		return 0, errInvalidQuotaPerUnit
	}
	price := decimal.NewFromFloat(priceAmount)
	unit := decimal.NewFromFloat(quotaPerUnit)
	var quota decimal.Decimal
	switch currency {
	case PlanCurrencyUSD:
		quota = price.Mul(unit).Ceil()
	case PlanCurrencyCNY:
		if !salePricePositive(salePrice) {
			return 0, errInvalidSalePrice
		}
		quota = price.Div(decimal.NewFromFloat(salePrice)).Mul(unit).Ceil()
	default:
		return 0, errUnsupportedPlanCurrency
	}
	return common.QuotaFromDecimalStrict(quota)
}

func computeDueDisplay(priceAmount float64, settlementCurrency string, salePrice float64, displayType string) (SubscriptionDueDisplay, error) {
	rounded := moneyAmount(priceAmount)
	display := strings.ToUpper(strings.TrimSpace(displayType))
	if display != PlanCurrencyUSD && display != PlanCurrencyCNY {
		return SubscriptionDueDisplay{Amount: rounded, Currency: settlementCurrency}, nil
	}
	if display == settlementCurrency {
		return SubscriptionDueDisplay{Amount: rounded, Currency: settlementCurrency}, nil
	}
	if !salePricePositive(salePrice) {
		return SubscriptionDueDisplay{}, errInvalidSalePrice
	}
	price := decimal.NewFromFloat(priceAmount)
	sale := decimal.NewFromFloat(salePrice)
	note := formatSettlementNote(settlementCurrency, priceAmount)
	if settlementCurrency == PlanCurrencyUSD && display == PlanCurrencyCNY {
		amount, _ := price.Mul(sale).Round(2).Float64()
		return SubscriptionDueDisplay{Amount: amount, Currency: PlanCurrencyCNY, NoteSettlement: &note}, nil
	}
	amount, _ := price.Div(sale).Round(2).Float64()
	return SubscriptionDueDisplay{Amount: amount, Currency: PlanCurrencyUSD, NoteSettlement: &note}, nil
}

func salePricePositive(salePrice float64) bool {
	return decimal.NewFromFloat(salePrice).GreaterThan(decimal.Zero)
}

func moneyAmount(priceAmount float64) float64 {
	amount, _ := decimal.NewFromFloat(priceAmount).Round(2).Float64()
	return amount
}

func formatSettlementNote(currency string, priceAmount float64) string {
	amount := moneyAmount(priceAmount)
	switch currency {
	case PlanCurrencyCNY:
		return fmt.Sprintf("¥%.2f", amount)
	default:
		return fmt.Sprintf("$%.2f", amount)
	}
}
