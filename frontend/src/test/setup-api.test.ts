import { afterEach, describe, expect, it, vi } from 'vitest';

import { createSetupApi, tokenFromLocation } from '@/lib/setup/setupApi';
import { waitForPanel } from '@/lib/setup/waitForPanel';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('tokenFromLocation', () => {
  it('reads the t parameter and is empty when absent', () => {
    expect(tokenFromLocation('?t=abc123&x=1')).toBe('abc123');
    expect(tokenFromLocation('')).toBe('');
  });
});

describe('createSetupApi', () => {
  it('maps a stable server code, and an unknown message collapses to network', async () => {
    const replies = [
      { success: false, msg: 'rate_limited' },
      { success: false, msg: 'some unlisted text' },
    ];
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ json: async () => replies.shift() }) as Response),
    );
    const api = createSetupApi('tok');
    expect(await api.checkPort(2053)).toEqual({ ok: false, code: 'rate_limited' });
    expect(await api.checkPort(2053)).toEqual({ ok: false, code: 'network' });
  });

  it('reports a transport failure as network instead of throwing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Promise.reject(new TypeError('offline'))),
    );
    expect(await createSetupApi('tok').status()).toEqual({ ok: false, code: 'network' });
  });
});

describe('waitForPanel', () => {
  it('resolves true once the probe starts answering, retrying every interval', async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('down'))
      .mockRejectedValueOnce(new TypeError('down'))
      .mockResolvedValue({});
    vi.stubGlobal('fetch', fetchMock);
    const result = waitForPanel('http://h:1/x/', { intervalMs: 1500, timeoutMs: 40000 });
    await vi.advanceTimersByTimeAsync(3000);
    expect(await result).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock).toHaveBeenCalledWith('http://h:1/x/', { mode: 'no-cors', cache: 'no-store' });
  });

  it('gives up with false after the timeout when nothing answers', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('down')));
    const result = waitForPanel('http://h:1/x/', { intervalMs: 1500, timeoutMs: 6000 });
    await vi.advanceTimersByTimeAsync(7500);
    expect(await result).toBe(false);
  });
});
