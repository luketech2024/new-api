import { describe, expect, it } from 'vitest'

import {
  canDecreaseCustomTopup,
  customTopupAmountPrefix,
  formatTopupPayCNY,
  isPositiveSalePrice,
  stepCustomTopupAmount,
} from './format'

describe('top-up pay formatting', () => {
  it('formats finite CNY pay amounts with a yuan sign', () => {
    expect(formatTopupPayCNY(73)).toBe('¥73.00')
  })

  it('hides pay amounts when the sale price is not a positive finite number', () => {
    expect(isPositiveSalePrice(0)).toBe(false)
    expect(isPositiveSalePrice(-1)).toBe(false)
    expect(isPositiveSalePrice(Number.NaN)).toBe(false)
    expect(formatTopupPayCNY(0)).toBe('-')
    expect(isPositiveSalePrice(7.3)).toBe(true)
  })
})

describe('custom top-up amount stepper', () => {
  it('steps by one internal unit and does not go below the minimum', () => {
    expect(stepCustomTopupAmount(7, 1, 1)).toBe(8)
    expect(stepCustomTopupAmount(7, -1, 1)).toBe(6)
    expect(stepCustomTopupAmount(1, -1, 1)).toBe(1)
    expect(stepCustomTopupAmount(0.5, -1, 1)).toBe(1)
    expect(stepCustomTopupAmount(Number.NaN, 1, 1)).toBe(1)
  })

  it('disables decrease only at or below the minimum', () => {
    expect(canDecreaseCustomTopup(7, 1)).toBe(true)
    expect(canDecreaseCustomTopup(1, 1)).toBe(false)
    expect(canDecreaseCustomTopup(0, 1)).toBe(false)
  })

  it('shows a currency prefix only for USD and CNY display types', () => {
    expect(customTopupAmountPrefix('USD')).toBe('$')
    expect(customTopupAmountPrefix('CNY')).toBe('¥')
    expect(customTopupAmountPrefix('TOKENS')).toBe('')
  })
})
