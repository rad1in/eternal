import { z } from 'zod';

import type { SetupErrorCode } from './validation';

export const SETUP_API_BASE = '/setup/api';
export const SETUP_TOKEN_HEADER = 'X-Setup-Token';

export type SetupCode = SetupErrorCode | 'network';
export type SetupResult<T> = { ok: true; data: T } | { ok: false; code: SetupCode };

const KNOWN_CODES: readonly SetupErrorCode[] = [
  'bad_token',
  'already_done',
  'username_invalid',
  'password_weak',
  'port_invalid',
  'port_in_use',
  'base_path_invalid',
  'rate_limited',
  'internal_error',
];

const envelope = z.object({
  success: z.boolean(),
  msg: z.string().optional(),
  obj: z.unknown().optional(),
});

const StatusSchema = z.object({
  pending: z.boolean(),
  suggestedPort: z.number(),
  suggestedBasePath: z.string(),
  serverHost: z.string(),
  version: z.string(),
});
const CheckPortSchema = z.object({ free: z.boolean() });
const FinishedSchema = z.object({ port: z.number(), basePath: z.string() });

export type SetupStatus = z.infer<typeof StatusSchema>;
export interface FinishInput {
  username: string;
  password: string;
  port: number;
  basePath: string;
  language: string;
}
export type SetupFinished = z.infer<typeof FinishedSchema>;

export function tokenFromLocation(search = window.location.search): string {
  return new URLSearchParams(search).get('t') ?? '';
}

// Unknown or unreadable failures collapse to 'network' so the UI has one generic message.
function toCode(msg: string | undefined): SetupCode {
  const known = KNOWN_CODES.find((c) => c === msg);
  return known ?? 'network';
}

export function createSetupApi(token: string) {
  async function call<T>(
    method: 'GET' | 'POST',
    path: string,
    schema: z.ZodType<T>,
    body?: unknown,
  ): Promise<SetupResult<T>> {
    try {
      const headers: Record<string, string> = { [SETUP_TOKEN_HEADER]: token };
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      const res = await fetch(`${SETUP_API_BASE}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        cache: 'no-store',
      });
      const parsed = envelope.safeParse(await res.json());
      if (!parsed.success) return { ok: false, code: 'network' };
      if (!parsed.data.success) return { ok: false, code: toCode(parsed.data.msg) };
      const data = schema.safeParse(parsed.data.obj);
      return data.success ? { ok: true, data: data.data } : { ok: false, code: 'network' };
    } catch {
      return { ok: false, code: 'network' };
    }
  }

  return {
    status: () => call('GET', '/status', StatusSchema),
    checkPort: (port: number) => call('POST', '/check-port', CheckPortSchema, { port }),
    finish: (input: FinishInput) => call('POST', '/finish', FinishedSchema, input),
  };
}

export type SetupApi = ReturnType<typeof createSetupApi>;
