import { describe, expect, it } from 'vitest';

import { launchInitData } from '@/lib/telegramApp';
import { customPrice, customSizeAllowed } from '@/lib/customOffer';

describe('Telegram Mini App launch data', () => {
  it('reads initData from the launch fragment exactly as Telegram encodes it', () => {
    const initData = 'query_id=AAH&user=%7B%22id%22%3A777%7D&auth_date=1700000000&hash=ab12';
    const hash = `#tgWebAppData=${encodeURIComponent(initData)}&tgWebAppVersion=8.0&tgWebAppPlatform=android`;
    expect(launchInitData(hash)).toBe(initData);
  });

  it('is empty in a normal browser', () => {
    expect(launchInitData('')).toBe('');
    expect(launchInitData('#section')).toBe('');
  });
});

const custom = {
  currency: 'IRT',
  pricePerGb: 2000,
  pricePerDay: 1000,
  minGb: 5,
  maxGb: 100,
  minDays: 7,
  maxDays: 90,
};

describe('calculator offer', () => {
  it('prices quota and days by their unit prices, as the server does', () => {
    expect(customPrice(custom, 20, 30)).toBe(70000);
    expect(customPrice(custom, 0, 7)).toBe(7000);
  });

  it('accepts a new or renewed service only when both sizes are in range', () => {
    expect(customSizeAllowed(custom, 'new', 20, 30)).toBe(true);
    expect(customSizeAllowed(custom, 'renew', 4, 30)).toBe(false);
    expect(customSizeAllowed(custom, 'new', 20, 91)).toBe(false);
    expect(customSizeAllowed(custom, 'new', 0, 30)).toBe(false);
  });

  it('lets an add-on add just one of the two, but not nothing', () => {
    expect(customSizeAllowed(custom, 'addon', 0, 7)).toBe(true);
    expect(customSizeAllowed(custom, 'addon', 10, 0)).toBe(true);
    expect(customSizeAllowed(custom, 'addon', 0, 0)).toBe(false);
    expect(customSizeAllowed(custom, 'addon', 3, 0)).toBe(false);
  });
});
