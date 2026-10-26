import type { ShopCustom } from '@/generated/zod';

export type CustomKind = 'new' | 'renew' | 'addon';

export function customPrice(
  c: Pick<ShopCustom, 'pricePerGb' | 'pricePerDay'>,
  gb: number,
  days: number,
): number {
  return gb * c.pricePerGb + days * c.pricePerDay;
}

// Mirrors the server: a new or renewed service needs both sizes in range, an
// add-on may add just one of them.
export function customSizeAllowed(
  c: ShopCustom,
  kind: CustomKind,
  gb: number,
  days: number,
): boolean {
  const gbOk = gb >= c.minGb && gb <= c.maxGb;
  const daysOk = days >= c.minDays && days <= c.maxDays;
  if (kind !== 'addon') return gbOk && daysOk;
  return (gb === 0 || gbOk) && (days === 0 || daysOk) && gb + days > 0;
}
