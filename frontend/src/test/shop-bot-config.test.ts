import { describe, expect, it } from 'vitest';

import {
  configToForm,
  formToConfig,
  joinToLines,
  linksToLines,
  parseJoinLines,
  parseLinkLines,
} from '@/lib/salesConfigForm';
import type { SalesConfigView } from '@/generated/zod';

describe('required channel and menu link lines', () => {
  it('reads one channel per line with an optional join link', () => {
    expect(parseJoinLines('@mychannel\n -1001234567890 | https://t.me/+AbCd \n\n')).toEqual([
      { chat: '@mychannel', link: '' },
      { chat: '-1001234567890', link: 'https://t.me/+AbCd' },
    ]);
  });

  it('round-trips channels and links through their text form', () => {
    const channels = [
      { chat: '@a_channel', link: 'https://t.me/a_channel' },
      { chat: '-100123456', link: 'https://t.me/+x' },
    ];
    expect(parseJoinLines(joinToLines(channels))).toEqual(channels);
    const links = [{ label: 'Tutorials', url: 'https://t.me/tutorials' }];
    expect(parseLinkLines(linksToLines(links))).toEqual(links);
  });

  it('keeps a pipe inside a URL and drops a line without both parts', () => {
    expect(
      parseLinkLines('Docs | https://example.com/a?b=c|d\nbroken line\n | https://x.y'),
    ).toEqual([{ label: 'Docs', url: 'https://example.com/a?b=c|d' }]);
  });
});

const view = {
  publicEnabled: true,
  publicBaseUrl: 'https://shop.example.com',
  card: {
    enabled: false,
    cardNumber: '',
    holderName: '',
    bankName: '',
    instructions: '',
  },
  zarinpal: { enabled: false, merchantId: '', sandbox: false },
  idpay: { enabled: false, apiKey: '', sandbox: false },
  crypto: { enabled: false, apiKey: '', ipnSecret: '', sandbox: false },
  stripe: { enabled: false, secretKey: '', webhookSecret: '' },
  paypal: { enabled: false, clientId: '', clientSecret: '', sandbox: false },
  walletEnabled: false,
  telegramShop: true,
  referral: { enabled: false, bonusPercent: 10 },
  bot: {
    forceJoin: [{ chat: '@mychannel', link: 'https://t.me/mychannel' }],
    links: [{ label: 'Docs', url: 'https://example.com' }],
    receiptChatId: -1001234567890,
    miniApp: true,
  },
  custom: {
    enabled: true,
    templateId: 3,
    currency: 'USD',
    pricePerGb: 150,
    pricePerDay: 25,
    minGb: 5,
    maxGb: 500,
    minDays: 7,
    maxDays: 365,
    limitIp: 2,
  },
  hasCryptoApiKey: false,
  hasCryptoIpnSecret: false,
  hasIdpayApiKey: false,
  hasPaypalClientSecret: false,
  hasStripeSecretKey: false,
  hasStripeWebhookSecret: false,
  hasZarinpalMerchantId: false,
} as SalesConfigView;

describe('sales config form mapping', () => {
  it('shows unit prices in major units and the lists as text', () => {
    const form = configToForm(view);
    expect(form.custom.pricePerGb).toBe(1.5);
    expect(form.custom.pricePerDay).toBe(0.25);
    expect(form.bot.forceJoinText).toBe('@mychannel | https://t.me/mychannel');
    expect(form.bot.linksText).toBe('Docs | https://example.com');
    expect(form.bot.receiptChatId).toBe(-1001234567890);
  });

  it('saves what the form shows back in minor units, whatever the currency', () => {
    const form = configToForm(view);
    form.custom.currency = 'IRT';
    form.custom.pricePerGb = 2000;
    form.bot.forceJoinText = '@other';
    const cfg = formToConfig(form);
    expect(cfg.custom.pricePerGb).toBe(2000);
    expect(cfg.bot.forceJoin).toEqual([{ chat: '@other', link: '' }]);
    const usd = formToConfig(configToForm(view));
    expect(usd.custom.pricePerGb).toBe(150);
    expect(usd.custom.pricePerDay).toBe(25);
  });
});
