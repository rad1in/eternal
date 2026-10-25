import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import SetupPage from '@/pages/setup/SetupPage';
import { ThemeProvider } from '@/hooks/useTheme';

vi.mock('@/lib/setup/waitForPanel', () => ({
  waitForPanel: () => new Promise<boolean>(() => {}),
}));

const STATUS = {
  pending: true,
  suggestedPort: 41234,
  suggestedBasePath: '/k3j9x2m1q8/',
  serverHost: '203.0.113.7',
  version: 'v3.1.0',
};

interface Call {
  path: string;
  headers: Record<string, string>;
  body: unknown;
}

function stubServer(finish: unknown) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url);
      calls.push({
        path,
        headers: (init?.headers ?? {}) as Record<string, string>,
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      let reply: unknown = { success: false, msg: 'internal_error' };
      if (path === '/setup/api/status') reply = { success: true, msg: '', obj: STATUS };
      if (path === '/setup/api/check-port') reply = { success: true, msg: '', obj: { free: true } };
      if (path === '/setup/api/finish') reply = finish;
      return { json: async () => reply } as Response;
    }),
  );
  return calls;
}

function renderSetup() {
  return render(
    <ThemeProvider>
      <SetupPage />
    </ThemeProvider>,
  );
}

const next = () => screen.getByRole('button', { name: /Next$/ }) as HTMLButtonElement;

async function fillAccount(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Username'), 'alice');
  await user.type(screen.getByLabelText('Password'), 'correct-horse-9');
  await user.type(screen.getByLabelText('Confirm password'), 'correct-horse-9');
}

async function toReview(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Next' }));
  await fillAccount(user);
  await user.click(next());
  await waitFor(() => expect(next().disabled).toBe(false));
  await user.click(next());
  return screen.findByRole('button', { name: 'Create my panel' });
}

beforeEach(() => {
  window.history.replaceState(null, '', '/setup?t=tok-abc');
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('setup wizard', () => {
  it('blocks Next on step 2 until the username is valid', async () => {
    stubServer({ success: true, obj: { port: 41234, basePath: '/k3j9x2m1q8/' } });
    const user = userEvent.setup();
    renderSetup();
    await user.click(await screen.findByRole('button', { name: 'Next' }));

    const username = await screen.findByLabelText('Username');
    await user.type(username, 'a b');
    await user.type(screen.getByLabelText('Password'), 'correct-horse-9');
    await user.type(screen.getByLabelText('Confirm password'), 'correct-horse-9');
    await user.tab();
    expect(next().disabled).toBe(true);
    expect(screen.getByText(/The username must be 3 to 32 characters/)).toBeTruthy();

    await user.clear(username);
    await user.type(username, 'alice');
    expect(next().disabled).toBe(false);
  });

  it('sends the setup token header on every request', async () => {
    const calls = stubServer({ success: true, obj: { port: 41234, basePath: '/k3j9x2m1q8/' } });
    const user = userEvent.setup();
    renderSetup();
    await user.click(await toReview(user));
    await screen.findByText('Your panel is ready');

    const paths = calls.map((c) => c.path);
    expect(paths).toContain('/setup/api/status');
    expect(paths).toContain('/setup/api/check-port');
    expect(paths).toContain('/setup/api/finish');
    for (const call of calls) expect(call.headers['X-Setup-Token']).toBe('tok-abc');
  });

  it('returns to the port step with a field error when the server says port_in_use', async () => {
    stubServer({ success: false, msg: 'port_in_use' });
    const user = userEvent.setup();
    renderSetup();
    await user.click(await toReview(user));

    expect(((await screen.findByLabelText('Port')) as HTMLInputElement).value).toBe('41234');
    expect(screen.getByText('This port is already in use. Choose another one.')).toBeTruthy();
    expect(next().disabled).toBe(true);
  });

  it('returns to the account step when the server rejects the password', async () => {
    stubServer({ success: false, msg: 'password_weak' });
    const user = userEvent.setup();
    renderSetup();
    await user.click(await toReview(user));

    expect(await screen.findByLabelText('Password')).toBeTruthy();
    expect(screen.getByText(/The password is too weak/)).toBeTruthy();
  });

  it('shows the final URL after a successful finish', async () => {
    const calls = stubServer({ success: true, obj: { port: 52000, basePath: '/zz99/' } });
    const user = userEvent.setup();
    renderSetup();
    await user.click(await toReview(user));

    const link = await screen.findByTestId('setup-final-url');
    expect(link.getAttribute('href')).toBe(
      `${window.location.protocol}//${window.location.hostname}:52000/zz99/`,
    );
    const finish = calls.find((c) => c.path === '/setup/api/finish');
    expect(finish?.body).toEqual({
      username: 'alice',
      password: 'correct-horse-9',
      port: 41234,
      basePath: '/k3j9x2m1q8/',
      language: 'en-US',
    });
  });

  it('shows the invalid-link page when the token is rejected', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ json: async () => ({ success: false, msg: 'bad_token' }) }) as Response),
    );
    renderSetup();
    expect(await screen.findByText('This setup link is not valid')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Next' })).toBeNull();
  });
});
