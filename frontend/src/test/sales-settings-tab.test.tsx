import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import SalesSettingsTab from '@/pages/sales/SalesSettingsTab';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

const view = {
  publicEnabled: true,
  publicBaseUrl: 'https://shop.example.com',
  card: { enabled: false, cardNumber: '', holderName: '', bankName: '', instructions: '' },
  zarinpal: { enabled: true, merchantId: '', sandbox: false },
  idpay: { enabled: false, apiKey: '', sandbox: false },
  crypto: { enabled: false, apiKey: '', ipnSecret: '', sandbox: false },
  stripe: { enabled: false, secretKey: '', webhookSecret: '' },
  paypal: { enabled: false, clientId: '', clientSecret: '', sandbox: false },
  walletEnabled: false,
  telegramShop: false,
  referral: { enabled: false, bonusPercent: 10 },
  bot: { forceJoin: [], links: [], receiptChatId: 0, miniApp: false },
  custom: {
    enabled: false,
    templateId: 0,
    currency: 'IRT',
    pricePerGb: 0,
    pricePerDay: 0,
    minGb: 1,
    maxGb: 100,
    minDays: 7,
    maxDays: 365,
    limitIp: 0,
  },
  hasZarinpalMerchantId: true,
  hasIdpayApiKey: false,
  hasCryptoApiKey: false,
  hasCryptoIpnSecret: false,
  hasStripeSecretKey: false,
  hasStripeWebhookSecret: false,
  hasPaypalClientSecret: false,
};

function savedBodies() {
  return vi
    .mocked(HttpUtil.post)
    .mock.calls.filter(([url]) => url === '/panel/api/sales/config/save')
    .map(([, body]) => body as Record<string, unknown>);
}

async function renderLoaded() {
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url: string) => new Msg(true, '', url.includes('/panel/api/sales/config') ? view : {}),
  );
  vi.mocked(HttpUtil.post).mockClear();
  renderWithProviders(<SalesSettingsTab />);
  await screen.findByPlaceholderText('Saved. Leave blank to keep it');
}

describe('SalesSettingsTab secrets', () => {
  it('never shows a stored secret and saves it as blank so the server keeps it', async () => {
    await renderLoaded();
    const input = screen.getByPlaceholderText('Saved. Leave blank to keep it') as HTMLInputElement;
    expect(input.value).toBe('');

    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    const body = savedBodies()[0] as { zarinpal: { merchantId: string }; clearSecrets: string[] };
    expect(body.zarinpal.merchantId).toBe('');
    expect(body.clearSecrets).toEqual([]);
  });

  it('names the secret to empty only when the operator ticks remove', async () => {
    await renderLoaded();
    fireEvent.click(screen.getByLabelText('Remove the saved value'));
    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    expect((savedBodies()[0] as { clearSecrets: string[] }).clearSecrets).toEqual([
      'zarinpal.merchantId',
    ]);
  });

  it('saves the Telegram shop switch the operator turns on', async () => {
    await renderLoaded();
    fireEvent.click(screen.getByLabelText('Sell in the Telegram bot'));
    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    expect((savedBodies()[0] as { telegramShop: boolean }).telegramShop).toBe(true);
  });

  it('saves the channel and menu link lines as lists, and the Mini App switch', async () => {
    await renderLoaded();
    fireEvent.change(screen.getByPlaceholderText('@mychannel'), {
      target: { value: '@mychannel | https://t.me/mychannel\n-100123456 | https://t.me/+abc' },
    });
    fireEvent.change(screen.getByPlaceholderText(/Tutorials \|/), {
      target: { value: 'Tutorials | https://t.me/tutorials' },
    });
    fireEvent.click(screen.getByLabelText('Mini App button'));
    fireEvent.click(screen.getByText('Save'));
    await waitFor(() => expect(savedBodies()).toHaveLength(1));
    const body = savedBodies()[0] as {
      bot: { forceJoin: unknown[]; links: unknown[]; miniApp: boolean };
    };
    expect(body.bot.forceJoin).toEqual([
      { chat: '@mychannel', link: 'https://t.me/mychannel' },
      { chat: '-100123456', link: 'https://t.me/+abc' },
    ]);
    expect(body.bot.links).toEqual([{ label: 'Tutorials', url: 'https://t.me/tutorials' }]);
    expect(body.bot.miniApp).toBe(true);
  });

  it('shows the link customers open to buy, built from the public base URL', async () => {
    await renderLoaded();
    expect(screen.getByText('https://shop.example.com/shop/')).toBeTruthy();
  });
});
