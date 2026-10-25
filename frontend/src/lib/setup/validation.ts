export type SetupErrorCode =
  | 'username_invalid'
  | 'password_weak'
  | 'port_invalid'
  | 'port_in_use'
  | 'base_path_invalid'
  | 'bad_token'
  | 'already_done'
  | 'rate_limited'
  | 'internal_error';

export const PORT_MIN = 1024;
export const PORT_MAX = 65535;
export const USERNAME_MIN = 3;
export const USERNAME_MAX = 32;
export const PASSWORD_MIN = 8;
export const BASE_PATH_MIN = 4;
export const BASE_PATH_MAX = 64;

// Mirrors the server's deny list; the server stays the authority.
const COMMON_PASSWORDS = new Set([
  'admin',
  'administrator',
  'password',
  'password1',
  'password123',
  '12345678',
  '123456789',
  '1234567890',
  'qwerty123',
  'qwertyuiop',
  'admin123',
  'admin1234',
  'letmein123',
  'iloveyou1',
  'eternal123',
]);

const USERNAME_RE = /^[A-Za-z0-9_.-]+$/;
const BASE_PATH_RE = /^[A-Za-z0-9_/-]+$/;

export function validateUsername(username: string): SetupErrorCode | null {
  if (username.length < USERNAME_MIN || username.length > USERNAME_MAX) return 'username_invalid';
  return USERNAME_RE.test(username) ? null : 'username_invalid';
}

export function validatePassword(password: string, username: string): SetupErrorCode | null {
  if (password.length < PASSWORD_MIN) return 'password_weak';
  if (password === username) return 'password_weak';
  return COMMON_PASSWORDS.has(password.toLowerCase()) ? 'password_weak' : null;
}

export function validatePort(port: number): SetupErrorCode | null {
  return Number.isInteger(port) && port >= PORT_MIN && port <= PORT_MAX ? null : 'port_invalid';
}

export function normalizeBasePath(raw: string): string {
  let p = raw.trim();
  if (!p.startsWith('/')) p = `/${p}`;
  if (!p.endsWith('/')) p = `${p}/`;
  return p;
}

export function validateBasePath(raw: string): SetupErrorCode | null {
  const p = normalizeBasePath(raw);
  if (p.length < BASE_PATH_MIN || p.length > BASE_PATH_MAX) return 'base_path_invalid';
  if (p.includes('//')) return 'base_path_invalid';
  return BASE_PATH_RE.test(p) ? null : 'base_path_invalid';
}

export function previewUrl(host: string, port: number, basePath: string): string {
  const bracketed = host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  return `http://${bracketed}:${port}${normalizeBasePath(basePath)}`;
}

// 0 = unacceptable, 1 weak, 2 fair, 3 good, 4 strong.
export function passwordStrength(password: string, username = ''): 0 | 1 | 2 | 3 | 4 {
  if (validatePassword(password, username) !== null) return 0;
  const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((re) =>
    re.test(password),
  ).length;
  const distinct = new Set(password).size;
  if (distinct < 4) return 1;
  let score = 1;
  if (password.length >= 12) score += 1;
  if (password.length >= 16) score += 1;
  if (classes >= 3) score += 1;
  if (classes < 2) score = Math.min(score, 2);
  return Math.min(score, 4) as 1 | 2 | 3 | 4;
}

function randomInt(maxExclusive: number): number {
  // Rejection sampling keeps the draw unbiased.
  const limit = Math.floor(0x100000000 / maxExclusive) * maxExclusive;
  const buf = new Uint32Array(1);
  do {
    crypto.getRandomValues(buf);
  } while (buf[0] >= limit);
  return buf[0] % maxExclusive;
}

const LOWER = 'abcdefghijkmnopqrstuvwxyz';
const UPPER = 'ABCDEFGHJKLMNPQRSTUVWXYZ';
const DIGITS = '23456789';
const SYMBOLS = '-_.!@#%*+=';

export function generatePassword(length = 20): string {
  const groups = [LOWER, UPPER, DIGITS, SYMBOLS];
  const all = groups.join('');
  const chars = groups.map((g) => g[randomInt(g.length)]);
  while (chars.length < length) chars.push(all[randomInt(all.length)]);
  for (let i = chars.length - 1; i > 0; i -= 1) {
    const j = randomInt(i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join('');
}

export function randomPort(): number {
  return PORT_MIN + 1000 + randomInt(PORT_MAX - PORT_MIN - 1000 + 1);
}

export function randomBasePath(): string {
  const alphabet = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let s = '';
  for (let i = 0; i < 10; i += 1) s += alphabet[randomInt(alphabet.length)];
  return `/${s}/`;
}
