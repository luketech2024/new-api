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

import type { PlanRecord } from '../types'

export function formatSettlementMoney(
  amount: number | undefined,
  currency: string | undefined
): string {
  const n = Number(amount)
  if (!Number.isFinite(n)) return '-'
  const body = n.toFixed(2)
  return currency === 'CNY' ? `¥${body}` : `$${body}`
}

export function planPurchaseDisabled(plan: PlanRecord | null | undefined): boolean {
  return plan?.balance?.ok === false
}

export function planDueLabel(plan: PlanRecord | null | undefined): string {
  if (!plan?.due_display) {
    return formatSettlementMoney(plan?.plan.price_amount, plan?.plan.currency)
  }
  return formatSettlementMoney(plan.due_display.amount, plan.due_display.currency)
}
