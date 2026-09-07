/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { DEFAULT_DISCOUNT_RATE } from '../constants'

// ============================================================================
// Wallet-specific Formatting Functions
// ============================================================================

/**
 * Format Creem price with currency symbol (USD/EUR)
 */
export function formatCreemPrice(
  price: number,
  currency: 'USD' | 'EUR'
): string {
  const symbol = currency === 'EUR' ? '€' : '$'
  return `${symbol}${price.toFixed(2)}`
}

/**
 * Format large quota numbers with K/M suffix
 */
export function formatQuotaShort(quota: number): string {
  if (quota >= 1000000) {
    return `${(quota / 1000000).toFixed(1)}M`
  }
  if (quota >= 1000) {
    return `${(quota / 1000).toFixed(1)}K`
  }
  return quota.toString()
}

/**
 * Format currency amount that is already in local currency.
 * This is used for payment amounts that have been calculated via priceRatio.
 */
export function formatCurrency(amount: number | string): string {
  const numeric =
    typeof amount === 'number' ? amount : Number.parseFloat(String(amount))
  if (!Number.isFinite(numeric)) return '-'

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: Math.abs(numeric) >= 1 ? 2 : 4,
  }).format(numeric)
}

export function formatTopupPayCNY(amount: number | string): string {
  const numeric =
    typeof amount === 'number' ? amount : Number.parseFloat(String(amount))
  if (!Number.isFinite(numeric) || numeric <= 0) return '-'
  return `¥${numeric.toFixed(2)}`
}

export function isPositiveSalePrice(priceRatio: number | undefined): boolean {
  return typeof priceRatio === 'number' && Number.isFinite(priceRatio) && priceRatio > 0
}

/** Prefix for the custom top-up input; empty when the display type is not a currency. */
export function customTopupAmountPrefix(
  quotaDisplayType: string | undefined
): string {
  if (quotaDisplayType === 'CNY') return '¥'
  if (quotaDisplayType === 'USD') return '$'
  return ''
}

/**
 * Step the custom top-up by one internal unit (USD display: $1; CNY display: one
 * dollar's worth at the display rate). Does not change pay-money formulas.
 */
export function stepCustomTopupAmount(
  currentInternal: number,
  delta: 1 | -1,
  minInternal: number
): number {
  const current = Number.isFinite(currentInternal) ? currentInternal : 0
  const min = Number.isFinite(minInternal) && minInternal > 0 ? minInternal : 0
  return Math.max(min, current + delta)
}

export function canDecreaseCustomTopup(
  currentInternal: number,
  minInternal: number
): boolean {
  const current = Number.isFinite(currentInternal) ? currentInternal : 0
  const min = Number.isFinite(minInternal) && minInternal > 0 ? minInternal : 0
  return current > min
}

/**
 * Get discount label for display (e.g., "20% OFF")
 */
export function getDiscountLabel(discount: number): string {
  if (discount >= DEFAULT_DISCOUNT_RATE) {
    return ''
  }
  const off = Math.round((1 - discount) * 100)
  return `${off}% OFF`
}

/**
 * Calculate pricing details for a preset amount
 */
export function calculatePresetPricing(
  presetValue: number,
  priceRatio: number,
  discount: number,
  usdExchangeRate: number = 1
) {
  const originalPrice = presetValue * priceRatio
  const actualPrice = originalPrice * discount
  const savedAmount = originalPrice - actualPrice
  const hasDiscount = discount < 1.0
  const displayValue = presetValue * usdExchangeRate

  return {
    displayValue,
    originalPrice,
    actualPrice,
    savedAmount,
    hasDiscount,
  }
}
