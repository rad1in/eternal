import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Input,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { CopyOutlined, EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';

import type {
  CarrierResult,
  PortStat,
  SpeedResult,
  TunnelEndView,
  TunnelView,
} from '@/generated/zod';
import {
  useTunnelAccess,
  useTunnelDetail,
  useTunnelLogs,
  useTunnelMutations,
} from '@/api/queries/useTunnels';
import { buildPorts, stateColor } from '@/lib/tunnel';
import type { TunnelPortValues } from '@/schemas/tunnel';
import { SizeFormatter } from '@/utils';
import PortsEditor from './PortsEditor';

interface TunnelDrawerProps {
  pair: string | null;
  canSeeToken: boolean;
  onClose: () => void;
}

const toValues = (ports: TunnelView['ports']): TunnelPortValues[] =>
  ports.map((p) => ({
    listen: p.listen,
    target: p.target,
    udp: p.udp,
    rateMBps: p.rateMBps ?? 0,
    quotaGB: p.quotaGB ?? 0,
  }));

function hostLabel(end: TunnelEndView, t: (k: string) => string): string {
  return end.host === 'node' ? end.nodeName || `#${end.nodeId}` : t('tunnel.host.local');
}

export default function TunnelDrawer({ pair, canSeeToken, onClose }: TunnelDrawerProps) {
  return (
    <Drawer
      open={pair != null}
      onClose={onClose}
      size="large"
      title={<DrawerTitle pair={pair} />}
      destroyOnHidden
    >
      {pair != null && (
        <DrawerBody key={pair} pair={pair} canSeeToken={canSeeToken} onClose={onClose} />
      )}
    </Drawer>
  );
}

function DrawerTitle({ pair }: { pair: string | null }) {
  const { t } = useTranslation();
  const detail = useTunnelDetail(pair);
  return <>{detail.data?.name ?? t('tunnel.title')}</>;
}

function DrawerBody({
  pair,
  canSeeToken,
  onClose,
}: {
  pair: string;
  canSeeToken: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const detail = useTunnelDetail(pair);
  const view = detail.data;
  const { update, remove, restart, reprobe, speedtest } = useTunnelMutations();

  const [tab, setTab] = useState('overview');
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<TunnelPortValues[]>([]);
  const [speed, setSpeed] = useState<SpeedResult | null>(null);
  const [logRole, setLogRole] = useState('iran');
  const [reveal, setReveal] = useState(false);

  const roles = useMemo(
    () => [view?.kharej && 'kharej', view?.iran && 'iran'].filter((r): r is string => !!r),
    [view],
  );
  const shownRole = roles.includes(logRole) ? logRole : (roles[0] ?? logRole);

  const logs = useTunnelLogs(pair, shownRole, tab === 'logs');
  const canConnect = canSeeToken && !!view?.kharej;
  const access = useTunnelAccess(pair, tab === 'connect' && canConnect);

  const live = view?.iran?.live ?? view?.kharej?.live ?? null;
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      messageApi.success(t('copied'));
    } catch {
      messageApi.error(t('somethingWentWrong'));
    }
  };

  const savePorts = async () => {
    if (!pair) return;
    const ok = await update.mutateAsync({ pair, body: { ports: buildPorts(draft) } });
    if (ok) {
      messageApi.success(t('tunnel.saved'));
      setEditing(false);
      void detail.refetch();
    }
  };

  const portColumns = [
    { title: t('tunnel.ports.listen'), dataIndex: 'port' },
    { title: '↑', dataIndex: 'bytesUp', render: (b: number) => SizeFormatter.sizeFormat(b) },
    { title: '↓', dataIndex: 'bytesDown', render: (b: number) => SizeFormatter.sizeFormat(b) },
    { title: t('tunnel.connections'), dataIndex: 'connsActive' },
    {
      title: t('tunnel.rate'),
      render: (_: unknown, r: PortStat) => SizeFormatter.speedFormat(r.rateUp + r.rateDown),
    },
    {
      title: t('tunnel.ports.quota'),
      render: (_: unknown, r: PortStat) =>
        r.quotaLimit > 0 ? (
          <Tag color={r.quotaExceeded ? 'error' : 'default'}>
            {SizeFormatter.sizeFormat(r.quotaUsed)} / {SizeFormatter.sizeFormat(r.quotaLimit)}
          </Tag>
        ) : (
          '-'
        ),
    },
  ];

  const probeColumns = [
    { title: t('tunnel.carrier'), dataIndex: 'carrier' },
    {
      title: t('tunnel.state.title'),
      render: (_: unknown, r: CarrierResult) =>
        r.ok ? <Tag color="success">{t('tunnel.state.up')}</Tag> : <Tag>{r.why || '-'}</Tag>,
    },
    {
      title: t('tunnel.speed'),
      render: (_: unknown, r: CarrierResult) =>
        r.ok ? `${r.downMBps.toFixed(1)} / ${r.upMBps.toFixed(1)} MB/s` : '-',
    },
  ];

  const overview = view && (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Space wrap>
        <Switch
          checked={(view.iran ?? view.kharej)?.enable ?? false}
          checkedChildren={t('enabled')}
          unCheckedChildren={t('disabled')}
          loading={update.isPending}
          onChange={(enable) => pair && update.mutate({ pair, body: { enable } })}
        />
        <Button
          onClick={() =>
            pair &&
            restart.mutate(pair, {
              onSuccess: (ok) => ok && messageApi.success(t('tunnel.restarted')),
            })
          }
          loading={restart.isPending}
        >
          {t('tunnel.restart')}
        </Button>
        <Button
          loading={speedtest.isPending}
          disabled={!view.iran}
          onClick={() => pair && speedtest.mutate(pair, { onSuccess: setSpeed })}
        >
          {t('tunnel.speedTest')}
        </Button>
        {view.auto && view.iran && (
          <Button
            loading={reprobe.isPending}
            onClick={() =>
              pair &&
              reprobe.mutate(pair, {
                onSuccess: (ok) => ok && messageApi.success(t('tunnel.reprobed')),
              })
            }
          >
            {t('tunnel.reprobe')}
          </Button>
        )}
        <Popconfirm
          title={t('tunnel.deleteConfirm', { name: view.name })}
          okText={t('delete')}
          cancelText={t('cancel')}
          okButtonProps={{ danger: true }}
          onConfirm={async () => {
            if (pair && (await remove.mutateAsync({ pair, force: false }))) onClose();
          }}
        >
          <Button danger loading={remove.isPending}>
            {t('delete')}
          </Button>
        </Popconfirm>
      </Space>
      {speed && (
        <Alert
          type={speed.error ? 'error' : 'success'}
          showIcon
          title={
            speed.error
              ? speed.error
              : `${t('tunnel.speedResult')}: ↓ ${speed.downMBps.toFixed(1)} MB/s  ↑ ${speed.upMBps.toFixed(1)} MB/s`
          }
        />
      )}
      <Descriptions
        size="small"
        bordered
        column={1}
        items={[
          {
            key: 'state',
            label: t('tunnel.state.title'),
            children: <Tag color={stateColor(view.state)}>{t(`tunnel.state.${view.state}`)}</Tag>,
          },
          {
            key: 'carrier',
            label: t('tunnel.carrier'),
            children: `${view.carrier}${view.auto && view.carrier !== 'auto' ? ` (${t('tunnel.auto')})` : ''}${view.reverse ? ` · ${t('tunnel.reverse')}` : ''}`,
          },
          ...([view.kharej, view.iran].filter(Boolean) as TunnelEndView[]).map((end) => ({
            key: end.uuid,
            label: t(`tunnel.role.${end.role}`),
            children: (
              <Space orientation="vertical" size={2}>
                <span>
                  {hostLabel(end, t)}{' '}
                  <Tag color={stateColor(end.state)}>{t(`tunnel.state.${end.state}`)}</Tag>
                  {end.rttMs > 0 && <Tag>{Math.round(end.rttMs)} ms</Tag>}
                </span>
                {end.lastError && <Typography.Text type="danger">{end.lastError}</Typography.Text>}
                {end.live && end.live.running && (
                  <Typography.Text type="secondary">
                    v{end.live.version} · {t('tunnel.uptime')} {Math.floor(end.live.uptimeSec / 60)}{' '}
                    min · ↓ {SizeFormatter.sizeFormat(end.live.bytesIn)} ↑{' '}
                    {SizeFormatter.sizeFormat(end.live.bytesOut)}
                  </Typography.Text>
                )}
              </Space>
            ),
          })),
          ...(view.external
            ? [
                {
                  key: 'external',
                  label: t('tunnel.host.external'),
                  children: t(`tunnel.role.${view.external}`),
                },
              ]
            : []),
        ]}
      />
      {view.probe.length > 0 && (
        <>
          <Typography.Title level={5}>{t('tunnel.probeResults')}</Typography.Title>
          <Table
            size="small"
            pagination={false}
            rowKey="carrier"
            dataSource={view.probe}
            columns={probeColumns}
          />
        </>
      )}
    </Space>
  );

  const portsTab = view && (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      {editing ? (
        <>
          <Alert type="info" showIcon title={t('tunnel.ports.hotReload')} />
          <PortsEditor value={draft} onChange={setDraft} />
          <Space>
            <Button type="primary" loading={update.isPending} onClick={savePorts}>
              {t('save')}
            </Button>
            <Button onClick={() => setEditing(false)}>{t('cancel')}</Button>
          </Space>
        </>
      ) : (
        <>
          <Button
            onClick={() => {
              setDraft(toValues(view.ports));
              setEditing(true);
            }}
          >
            {t('tunnel.ports.edit')}
          </Button>
          <Table
            size="small"
            pagination={false}
            rowKey="port"
            dataSource={live?.ports ?? []}
            columns={portColumns}
            scroll={{ x: 'max-content' }}
            locale={{
              emptyText: view.ports
                .map((p) => `${p.listen}→${p.target}${p.udp ? '/udp' : ''}`)
                .join(', '),
            }}
          />
        </>
      )}
    </Space>
  );

  const logsTab = (
    <Space orientation="vertical" size="small" style={{ width: '100%' }}>
      {roles.length > 1 && (
        <Select
          value={shownRole}
          onChange={setLogRole}
          options={roles.map((r) => ({ value: r, label: t(`tunnel.role.${r}`) }))}
          style={{ minWidth: 200 }}
        />
      )}
      <pre className="tunnel-log">{(logs.data ?? []).join('\n') || t('noData')}</pre>
    </Space>
  );

  const connectTab = (
    <Spin spinning={access.isFetching}>
      {access.data ? (
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          <Alert type="warning" showIcon title={t('tunnel.connect.warning')} />
          <Typography.Text strong>{t('tunnel.connect.command')}</Typography.Text>
          <Input.TextArea
            readOnly
            rows={4}
            className="tunnel-command"
            value={
              reveal
                ? access.data.command
                : access.data.command.replace(/wat1:[A-Za-z0-9+/=]+/, 'wat1:••••••••')
            }
          />
          <Space>
            <Button
              icon={reveal ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              onClick={() => setReveal(!reveal)}
            >
              {reveal ? t('tunnel.connect.hide') : t('tunnel.connect.show')}
            </Button>
            <Button icon={<CopyOutlined />} onClick={() => copy(access.data!.command)}>
              {t('copy')}
            </Button>
            <Button icon={<CopyOutlined />} onClick={() => copy(access.data!.token)}>
              {t('tunnel.connect.copyToken')}
            </Button>
          </Space>
        </Space>
      ) : (
        <Typography.Text type="secondary">
          {access.isError ? t('tunnel.connect.none') : ''}
        </Typography.Text>
      )}
    </Spin>
  );

  return (
    <>
      {messageContextHolder}
      {detail.isError && <Alert type="error" showIcon title={(detail.error as Error).message} />}
      <Spin spinning={detail.isLoading}>
        {view && (
          <Tabs
            activeKey={tab}
            onChange={setTab}
            items={[
              { key: 'overview', label: t('tunnel.tab.overview'), children: overview },
              { key: 'ports', label: t('tunnel.portsTitle'), children: portsTab },
              { key: 'logs', label: t('tunnel.tab.logs'), children: logsTab },
              ...(canConnect
                ? [{ key: 'connect', label: t('tunnel.tab.connect'), children: connectTab }]
                : []),
            ]}
          />
        )}
      </Spin>
    </>
  );
}
