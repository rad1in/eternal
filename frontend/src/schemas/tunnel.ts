import { z } from 'zod';

export const END_KINDS = ['local', 'node', 'external'] as const;
export type EndKind = (typeof END_KINDS)[number];

export const TUNNEL_CARRIERS = [
  'tcp',
  'udp',
  'tls',
  'ws',
  'wss',
  'h2',
  'h2c',
  'grpc',
  'icmp',
] as const;
export const POOL_CARRIERS = ['tcp', 'tls', 'ws', 'wss', 'h2', 'h2c', 'grpc'] as const;
export const FAKE_MODES = [
  'seq',
  'ttl',
  'sum',
  'seqsum',
  'rst',
  'auto',
  'split',
  'hlen0',
  'fakeip',
  'synrst',
  'all',
  'ff',
] as const;
export const TLS_FINGERPRINTS = ['go', 'chrome', 'firefox', 'safari', 'edge', 'random'] as const;

export const tunnelPortSchema = z.object({
  listen: z.number().int().min(1, 'tunnel.errPortRange').max(65535, 'tunnel.errPortRange'),
  target: z.number().int().min(1, 'tunnel.errPortRange').max(65535, 'tunnel.errPortRange'),
  udp: z.boolean(),
  rateMBps: z.number().min(0),
  quotaGB: z.number().min(0),
});
export type TunnelPortValues = z.infer<typeof tunnelPortSchema>;

const HOST_NAME = /^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$/;
const SIZE_OR_TIME = /^[0-9]+(\.[0-9]+)?([a-zA-Z]{1,2})?$/;

export const tunnelWizardSchema = z
  .object({
    name: z
      .string()
      .trim()
      .regex(/^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$/, 'tunnel.errName'),
    kharejKind: z.enum(END_KINDS),
    kharejNodeId: z.number().int().nullable(),
    kharejAddress: z.string().trim(),
    iranKind: z.enum(END_KINDS),
    iranNodeId: z.number().int().nullable(),
    iranAddress: z.string().trim(),
    token: z.string().trim(),
    ports: z.array(tunnelPortSchema).min(1, 'tunnel.errPorts'),
    carrier: z.string(),
    listenPort: z.number().int().min(0).max(65535),
    path: z.string().trim(),
    sni: z.string().trim(),
    reverse: z.boolean(),
    fakeSni: z
      .string()
      .trim()
      .refine((v) => v === '' || HOST_NAME.test(v), 'tunnel.errHost'),
    fakeMode: z.string(),
    poolRotate: z
      .string()
      .trim()
      .refine((v) => v === '' || SIZE_OR_TIME.test(v), 'tunnel.errRotate'),
    ech: z.string().trim(),
    fp: z.string(),
    tlsSmallCh: z.boolean(),
    maxMBps: z.number().int().min(0),
  })
  .superRefine((v, ctx) => {
    const issue = (path: string, message: string) =>
      ctx.addIssue({ code: 'custom', path: [path], message });
    for (const side of ['kharej', 'iran'] as const) {
      if (v[`${side}Kind`] === 'node' && v[`${side}NodeId`] == null) {
        issue(`${side}NodeId`, 'tunnel.errNode');
      }
    }
    if (v.kharejKind === 'external' && v.iranKind === 'external') {
      issue('iranKind', 'tunnel.errBothExternal');
    }
    if (v.kharejKind === v.iranKind && v.kharejKind !== 'external') {
      const sameNode = v.kharejKind === 'local' || v.kharejNodeId === v.iranNodeId;
      if (sameNode) issue('iranKind', 'tunnel.errSameHost');
    }
    if (v.kharejKind === 'external' && !v.token.startsWith('wat1:')) {
      issue('token', 'tunnel.errToken');
    }
    const seen = new Set<string>();
    v.ports.forEach((p, i) => {
      const k = `${p.listen}/${p.udp}`;
      if (seen.has(k)) issue(`ports.${i}.listen`, 'tunnel.errDupPort');
      seen.add(k);
    });
    if (v.reverse && v.carrier === 'auto') issue('carrier', 'tunnel.errReverseAuto');
    if (v.reverse && v.iranKind === 'external' && v.listenPort === 0) {
      issue('listenPort', 'tunnel.errReversePort');
    }
  });
export type TunnelWizardValues = z.infer<typeof tunnelWizardSchema>;
