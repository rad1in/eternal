import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import {
  SpeedResultSchema,
  TunnelAccessSchema,
  TunnelCapabilitiesSchema,
  TunnelViewSchema,
  type SpeedResult,
  type TunnelAccess,
  type TunnelCapabilities,
  type TunnelCreateRequest,
  type TunnelUpdateRequest,
  type TunnelView,
} from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const BASE = '/panel/api/tunnels';
const LIST_REFRESH_MS = 5000;
const DETAIL_REFRESH_MS = 3000;

export const CreatedSchema = z.object({
  tunnel: TunnelViewSchema.nullable(),
  access: TunnelAccessSchema.nullable(),
});
export type TunnelCreated = z.infer<typeof CreatedSchema>;

async function fetchList(): Promise<TunnelView[]> {
  const msg = await HttpUtil.get(`${BASE}/list`, undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load tunnels');
  return parseMsg(msg, z.array(TunnelViewSchema), 'tunnels/list').obj ?? [];
}

export function useTunnelList() {
  const query = useQuery({
    queryKey: keys.tunnels.list(),
    queryFn: fetchList,
    refetchInterval: LIST_REFRESH_MS,
  });
  return {
    tunnels: query.data ?? [],
    loading: query.isFetching,
    fetched: query.isFetched,
    error: query.error instanceof Error ? query.error.message : '',
    refetch: query.refetch,
  };
}

export function useTunnelOptions(): TunnelCapabilities | undefined {
  const query = useQuery({
    queryKey: keys.tunnels.options(),
    queryFn: async () => {
      const msg = await HttpUtil.get(`${BASE}/options`, undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load tunnel options');
      return parseMsg(msg, TunnelCapabilitiesSchema, 'tunnels/options').obj as TunnelCapabilities;
    },
    staleTime: 60_000,
  });
  return query.data;
}

export function useTunnelDetail(pair: string | null) {
  return useQuery({
    queryKey: keys.tunnels.detail(pair ?? ''),
    enabled: pair != null,
    refetchInterval: DETAIL_REFRESH_MS,
    queryFn: async () => {
      const msg = await HttpUtil.get(`${BASE}/get/${pair}`, undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the tunnel');
      return parseMsg(msg, TunnelViewSchema, 'tunnels/get').obj as TunnelView;
    },
  });
}

export function useTunnelLogs(pair: string | null, role: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.tunnels.logs(pair ?? '', role),
    enabled: enabled && pair != null,
    refetchInterval: DETAIL_REFRESH_MS,
    queryFn: async () => {
      const msg = await HttpUtil.get(
        `${BASE}/logs/${pair}/${role}`,
        { lines: 200 },
        { silent: true },
      );
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the log');
      return parseMsg(msg, z.array(z.string()), 'tunnels/logs').obj ?? [];
    },
  });
}

export function useTunnelAccess(pair: string | null, enabled: boolean) {
  return useQuery({
    queryKey: keys.tunnels.access(pair ?? ''),
    enabled: enabled && pair != null,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    queryFn: async () => {
      const msg = await HttpUtil.get(`${BASE}/token/${pair}`, undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'No token');
      return parseMsg(msg, TunnelAccessSchema, 'tunnels/token').obj as TunnelAccess;
    },
  });
}

export function useTunnelMutations() {
  const queryClient = useQueryClient();
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.tunnels.root() });

  const create = useMutation({
    mutationFn: async (req: TunnelCreateRequest): Promise<TunnelCreated | null> => {
      const msg = await HttpUtil.post(`${BASE}/create`, req, JSON_HEADERS);
      if (!msg?.success) return null;
      return parseMsg(msg, CreatedSchema, 'tunnels/create').obj as TunnelCreated;
    },
    onSuccess: () => void refresh(),
  });
  const update = useMutation({
    mutationFn: async (args: { pair: string; body: TunnelUpdateRequest }) => {
      const msg = await HttpUtil.post(`${BASE}/update/${args.pair}`, args.body, JSON_HEADERS);
      return msg?.success ?? false;
    },
    onSuccess: () => void refresh(),
  });
  const remove = useMutation({
    mutationFn: async (args: { pair: string; force: boolean }) => {
      const msg = await HttpUtil.post(
        `${BASE}/del/${args.pair}`,
        { force: args.force },
        JSON_HEADERS,
      );
      return msg?.success ?? false;
    },
    onSuccess: () => void refresh(),
  });
  const restart = useMutation({
    mutationFn: async (pair: string) =>
      (await HttpUtil.post(`${BASE}/restart/${pair}`))?.success ?? false,
    onSuccess: () => void refresh(),
  });
  const reprobe = useMutation({
    mutationFn: async (pair: string) =>
      (await HttpUtil.post(`${BASE}/reprobe/${pair}`))?.success ?? false,
    onSuccess: () => void refresh(),
  });
  const speedtest = useMutation({
    mutationFn: async (pair: string): Promise<SpeedResult | null> => {
      const msg = await HttpUtil.post(`${BASE}/speedtest/${pair}`, undefined, {
        params: { mb: 10 },
      });
      if (!msg?.success) return null;
      return parseMsg(msg, SpeedResultSchema, 'tunnels/speedtest').obj as SpeedResult;
    },
  });

  return { create, update, remove, restart, reprobe, speedtest };
}
