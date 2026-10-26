import { describe, expect, test } from 'vitest';

import {
  buildCreateRequest,
  defaultWizardValues,
  kharejSetupCommand,
  tunnelCounts,
} from '@/lib/tunnel';
import { tunnelWizardSchema, type TunnelWizardValues } from '@/schemas/tunnel';
import type { TunnelView } from '@/generated/zod';

function values(patch: Partial<TunnelWizardValues> = {}): TunnelWizardValues {
  return { ...defaultWizardValues(), name: 'de-ir', kharejNodeId: 2, ...patch };
}

function firstIssue(v: TunnelWizardValues): string | undefined {
  const r = tunnelWizardSchema.safeParse(v);
  return r.success ? undefined : r.error.issues[0]?.message;
}

describe('tunnel wizard validation', () => {
  test('the default node-to-panel tunnel is valid once a node is picked', () => {
    expect(tunnelWizardSchema.safeParse(values()).success).toBe(true);
  });

  test.each([
    ['a node end without a node', { kharejNodeId: null }, 'tunnel.errNode'],
    ['both ends on this panel', { kharejKind: 'local' as const }, 'tunnel.errSameHost'],
    [
      'both ends on one node',
      { kharejKind: 'node' as const, iranKind: 'node' as const, iranNodeId: 2 },
      'tunnel.errSameHost',
    ],
    [
      'both ends external',
      { kharejKind: 'external' as const, iranKind: 'external' as const, token: 'wat1:x' },
      'tunnel.errBothExternal',
    ],
    [
      'an external Kharej without its token',
      { kharejKind: 'external' as const },
      'tunnel.errToken',
    ],
    [
      'a public port mapped twice',
      {
        ports: [
          { listen: 443, target: 443, udp: false, rateMBps: 0, quotaGB: 0 },
          { listen: 443, target: 8443, udp: false, rateMBps: 0, quotaGB: 0 },
        ],
      },
      'tunnel.errDupPort',
    ],
    ['reverse with auto', { reverse: true }, 'tunnel.errReverseAuto'],
    ['a name with a newline', { name: 'a\nb' }, 'tunnel.errName'],
    ['no ports', { ports: [] }, 'tunnel.errPorts'],
    ['a fake SNI that is not a host name', { fakeSni: 'a b' }, 'tunnel.errHost'],
  ])('rejects %s', (_label, patch, message) => {
    expect(firstIssue(values(patch))).toBe(message);
  });

  test('tcp and udp may share a public port number', () => {
    const ports = [
      { listen: 53, target: 53, udp: false, rateMBps: 0, quotaGB: 0 },
      { listen: 53, target: 53, udp: true, rateMBps: 0, quotaGB: 0 },
    ];
    expect(tunnelWizardSchema.safeParse(values({ ports })).success).toBe(true);
  });

  test('reverse to a server without a panel needs the carrier port', () => {
    const base = { reverse: true, carrier: 'tcp', iranKind: 'external' as const };
    expect(firstIssue(values(base))).toBe('tunnel.errReversePort');
    expect(tunnelWizardSchema.safeParse(values({ ...base, listenPort: 24500 })).success).toBe(true);
  });
});

describe('buildCreateRequest', () => {
  test('keeps the daemon defaults when nothing is set', () => {
    const req = buildCreateRequest(values());
    expect(req.options).toEqual({});
    expect(req.ports).toEqual([{ listen: 443, target: 443, udp: false }]);
    expect(req.kharej).toEqual({ kind: 'node', nodeId: 2, address: '' });
    expect(req.iran).toEqual({ kind: 'local', nodeId: null, address: '' });
    expect(req.token).toBe('');
  });

  test('sends the anti-censorship kit and per-port limits only when they are set', () => {
    const req = buildCreateRequest(
      values({
        fakeSni: 'www.cloudflare.com',
        fakeMode: 'rst',
        poolRotate: '30m',
        tlsSmallCh: true,
        maxMBps: 40,
        ports: [{ listen: 443, target: 443, udp: false, rateMBps: 5, quotaGB: 100 }],
      }),
    );
    expect(req.options).toEqual({
      fakeSni: 'www.cloudflare.com',
      fakeMode: 'rst',
      poolRotate: '30m',
      tlsSmallCh: true,
      maxMBps: 40,
    });
    expect(req.ports[0]).toEqual({
      listen: 443,
      target: 443,
      udp: false,
      rateMBps: 5,
      quotaGB: 100,
    });
  });

  test('a fake mode without a fake SNI is not sent', () => {
    expect(buildCreateRequest(values({ fakeMode: 'rst' })).options).toEqual({});
  });

  test('the pasted token goes along only for an external Kharej', () => {
    const external = buildCreateRequest(values({ kharejKind: 'external', token: 'wat1:abc' }));
    expect(external.token).toBe('wat1:abc');
    expect(external.kharej.nodeId).toBeNull();
    expect(buildCreateRequest(values({ token: 'wat1:leftover' })).token).toBe('');
  });
});

describe('kharejSetupCommand', () => {
  test('lists the target ports once, with udp marked', () => {
    const cmd = kharejSetupCommand('https://panel.example/base/', {
      carrier: 'auto',
      kharejAddress: '203.0.113.7',
      name: 'de ir',
      ports: [
        { listen: 443, target: 443, udp: false, rateMBps: 0, quotaGB: 0 },
        { listen: 8443, target: 443, udp: false, rateMBps: 0, quotaGB: 0 },
        { listen: 53, target: 53, udp: true, rateMBps: 0, quotaGB: 0 },
      ],
    });
    expect(cmd).toBe(
      'curl -fsSL https://panel.example/base/install-tunnel.sh | bash -s -- --role kharej --addr 203.0.113.7 --carrier auto --ports 443,53/udp --name de-ir',
    );
  });
});

describe('tunnelCounts', () => {
  test('counts up and down links, treating error as down', () => {
    const mk = (state: string) => ({ state }) as TunnelView;
    expect(tunnelCounts([mk('up'), mk('down'), mk('error'), mk('stopped'), mk('unknown')])).toEqual(
      {
        total: 5,
        up: 1,
        down: 2,
      },
    );
  });
});
