import { describe, expect, it } from 'vitest'

import { formatTopupPayCNY, isPositiveSalePrice } from './format'

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
