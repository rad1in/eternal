import { describe, expect, it } from 'vitest';

import {
  generatePassword,
  normalizeBasePath,
  passwordStrength,
  previewUrl,
  randomBasePath,
  randomPort,
  validateBasePath,
  validatePassword,
  validatePort,
  validateUsername,
} from '@/lib/setup/validation';

describe('setup validateUsername', () => {
  it.each([
    ['ab', 'username_invalid'],
    ['abc', null],
    ['a'.repeat(32), null],
    ['a'.repeat(33), 'username_invalid'],
    ['john doe', 'username_invalid'],
    ['jöhn', 'username_invalid'],
    ['j.o-h_n9', null],
  ])('%s -> %s', (input, expected) => {
    expect(validateUsername(input)).toBe(expected);
  });
});

describe('setup validatePassword', () => {
  it.each([
    ['short1!', 'bob', 'password_weak'],
    ['12345678', 'bob', 'password_weak'],
    ['QWERTY123', 'bob', 'password_weak'],
    ['samename1', 'samename1', 'password_weak'],
    ['correct-horse-9', 'bob', null],
    ['abcdefgh', 'bob', null],
  ])('%s for %s -> %s', (pw, user, expected) => {
    expect(validatePassword(pw, user)).toBe(expected);
  });
});

describe('setup validatePort', () => {
  it.each([
    [1023, 'port_invalid'],
    [1024, null],
    [65535, null],
    [65536, 'port_invalid'],
    [8080.5, 'port_invalid'],
    [Number.NaN, 'port_invalid'],
  ])('%s -> %s', (port, expected) => {
    expect(validatePort(port)).toBe(expected);
  });
});

describe('setup base path', () => {
  it('adds the missing leading and trailing slash', () => {
    expect(normalizeBasePath('abcd')).toBe('/abcd/');
    expect(normalizeBasePath(' /abcd ')).toBe('/abcd/');
  });

  it.each([
    ['/a/', 'base_path_invalid'],
    ['/ab/', null],
    ['abc', null],
    ['/a b c/', 'base_path_invalid'],
    ['/ab//cd/', 'base_path_invalid'],
    ['/ab.cd/', 'base_path_invalid'],
    [`/${'a'.repeat(62)}/`, null],
    [`/${'a'.repeat(63)}/`, 'base_path_invalid'],
    ['/nested/path_ok-1/', null],
  ])('%s -> %s', (input, expected) => {
    expect(validateBasePath(input)).toBe(expected);
  });
});

describe('setup previewUrl', () => {
  it('joins host, port and normalised path', () => {
    expect(previewUrl('203.0.113.7', 2053, 'k3j9')).toBe('http://203.0.113.7:2053/k3j9/');
  });

  it('brackets an IPv6 host so the port is not read as part of the address', () => {
    expect(previewUrl('2001:db8::1', 8443, '/x1y2/')).toBe('http://[2001:db8::1]:8443/x1y2/');
  });
});

describe('setup passwordStrength', () => {
  it('scores an unacceptable password as 0 whatever its shape', () => {
    expect(passwordStrength('admin')).toBe(0);
    expect(passwordStrength('Abcdef1!', 'Abcdef1!')).toBe(0);
  });

  it('rates a repetitive password weak despite its length', () => {
    expect(passwordStrength('aaaaaaaaaaaaaaaa')).toBe(1);
  });

  it('rises with length and variety', () => {
    const fair = passwordStrength('abcdefgh');
    const good = passwordStrength('abcdefgh1234');
    const strong = passwordStrength('Abcdefgh1234!xyzQ');
    expect(fair).toBe(1);
    expect(good).toBe(2);
    expect(strong).toBe(4);
  });
});

describe('setup generators', () => {
  it('generatePassword yields a password the validator and meter accept as strong', () => {
    for (let i = 0; i < 50; i += 1) {
      const pw = generatePassword();
      expect(pw).toHaveLength(20);
      expect(validatePassword(pw, 'admin')).toBeNull();
      expect(passwordStrength(pw)).toBe(4);
      expect(
        /[a-z]/.test(pw) && /[A-Z]/.test(pw) && /[0-9]/.test(pw) && /[^A-Za-z0-9]/.test(pw),
      ).toBe(true);
    }
  });

  it('randomPort and randomBasePath always pass the validators', () => {
    for (let i = 0; i < 200; i += 1) {
      expect(validatePort(randomPort())).toBeNull();
      expect(validateBasePath(randomBasePath())).toBeNull();
    }
  });
});
