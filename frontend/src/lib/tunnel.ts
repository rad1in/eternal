import type {
  EndOptions,
  PortMap,
  TunnelCreateRequest,
  TunnelEndpoint,
  TunnelView,
} from '@/generated/zod';
import type { TunnelWizardValues } from '@/schemas/tunnel';

export function defaultWizardValues(): TunnelWizardValues {
  return {
    name: '',
    kharejKind: 'node',
    kharejNodeId: null,
    kharejAddress: '',
    iranKind: 'local',
    iranNodeId: null,
    iranAddress: '',
    token: '',
    ports: [{ listen: 443, target: 443, udp: false, rateMBps: 0, quotaGB: 0 }],
    carrier: 'auto',
    listenPort: 0,
    path: '',
    sni: '',
    reverse: false,
    fakeSni: '',
    fakeMode: '',
    poolRotate: '',
    ech: '',
    fp: '',
    tlsSmallCh: false,
    maxMBps: 0,
  };
}

function endpoint(kind: string, nodeId: number | null, address: string): TunnelEndpoint {
  return {
    kind,
    nodeId: kind === 'node' ? nodeId : null,
    address: address.trim(),
  };
}

// Zero and empty values are dropped so the daemon keeps its own defaults.
export function buildOptions(v: TunnelWizardValues): EndOptions {
  const o: EndOptions = {};
  if (v.maxMBps > 0) o.maxMBps = v.maxMBps;
  if (v.fakeSni) {
    o.fakeSni = v.fakeSni;
    if (v.fakeMode) o.fakeMode = v.fakeMode;
  }
  if (v.poolRotate) o.poolRotate = v.poolRotate;
  if (v.ech) o.ech = v.ech;
  if (v.fp) o.fp = v.fp;
  if (v.tlsSmallCh) o.tlsSmallCh = true;
  return o;
}

export function buildPorts(ports: TunnelWizardValues['ports']): PortMap[] {
  return ports.map((p) => {
    const out: PortMap = { listen: p.listen, target: p.target, udp: p.udp };
    if (p.rateMBps > 0) out.rateMBps = p.rateMBps;
    if (p.quotaGB > 0) out.quotaGB = p.quotaGB;
    return out;
  });
}

export function buildCreateRequest(v: TunnelWizardValues): TunnelCreateRequest {
  return {
    name: v.name.trim(),
    kharej: endpoint(v.kharejKind, v.kharejNodeId, v.kharejAddress),
    iran: endpoint(v.iranKind, v.iranNodeId, v.iranAddress),
    carrier: v.carrier,
    listenPort: v.listenPort,
    path: v.path,
    sni: v.sni,
    icmpType: '',
    reverse: v.reverse,
    ports: buildPorts(v.ports),
    options: buildOptions(v),
    token: v.kharejKind === 'external' ? v.token : '',
  };
}

// The command that makes the Kharej end on a server without a panel. It holds no secret.
export function kharejSetupCommand(
  origin: string,
  v: Pick<TunnelWizardValues, 'ports' | 'carrier' | 'kharejAddress' | 'name'>,
): string {
  const ports = v.ports.map((p) => `${p.target}${p.udp ? '/udp' : ''}`);
  const unique = [...new Set(ports)].join(',');
  const addr = v.kharejAddress.trim() || 'YOUR_SERVER_IP';
  const name = v.name.trim().replace(/[^A-Za-z0-9_-]+/g, '-');
  return (
    `curl -fsSL ${origin.replace(/\/+$/, '')}/install-tunnel.sh | bash -s -- --role kharej` +
    ` --addr ${addr} --carrier ${v.carrier} --ports ${unique}` +
    (name ? ` --name ${name}` : '')
  );
}

export type StateColor = 'success' | 'error' | 'processing' | 'default' | 'warning';

export function stateColor(state: string): StateColor {
  switch (state) {
    case 'up':
      return 'success';
    case 'down':
    case 'error':
      return 'error';
    case 'starting':
      return 'processing';
    case 'stopped':
      return 'default';
    default:
      return 'warning';
  }
}

export function tunnelCounts(list: readonly TunnelView[]): {
  total: number;
  up: number;
  down: number;
} {
  let up = 0;
  let down = 0;
  for (const t of list) {
    if (t.state === 'up') up += 1;
    else if (t.state === 'down' || t.state === 'error') down += 1;
  }
  return { total: list.length, up, down };
}
