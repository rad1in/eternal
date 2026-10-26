import { useTranslation } from 'react-i18next';
import { Button, Empty, Popconfirm, Space, Table, Tag, Tooltip, Typography } from 'antd';
import { ArrowRightOutlined, DeleteOutlined, EyeOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import type { TunnelEndView, TunnelView } from '@/generated/zod';
import { stateColor } from '@/lib/tunnel';

interface TunnelListProps {
  tunnels: TunnelView[];
  loading: boolean;
  onOpen: (t: TunnelView) => void;
  onRestart: (t: TunnelView) => void;
  onDelete: (t: TunnelView) => void;
}

function EndCell({ end, side }: { end: TunnelEndView | null | undefined; side: string }) {
  const { t } = useTranslation();
  if (!end) {
    return (
      <div className="route-end">
        <span>{t('tunnel.host.external')}</span>
        <small>{t(`tunnel.role.${side}`)}</small>
      </div>
    );
  }
  const host = end.host === 'node' ? end.nodeName || `#${end.nodeId}` : t('tunnel.host.local');
  return (
    <div className="route-end">
      <span>{host}</span>
      <small>{t(`tunnel.role.${side}`)}</small>
    </div>
  );
}

export default function TunnelList({
  tunnels,
  loading,
  onOpen,
  onRestart,
  onDelete,
}: TunnelListProps) {
  const { t } = useTranslation();

  const columns: ColumnsType<TunnelView> = [
    {
      title: t('tunnel.name'),
      dataIndex: 'name',
      render: (name: string, row) => (
        <Typography.Link onClick={() => onOpen(row)}>{name}</Typography.Link>
      ),
    },
    {
      title: t('tunnel.route'),
      key: 'route',
      render: (_, row) => (
        <div className="route-cell">
          <EndCell end={row.kharej} side="kharej" />
          <ArrowRightOutlined style={{ opacity: 0.5 }} />
          <EndCell end={row.iran} side="iran" />
        </div>
      ),
    },
    {
      title: t('tunnel.carrier'),
      key: 'carrier',
      render: (_, row) => (
        <Space size={4}>
          <Tag>{row.carrier}</Tag>
          {row.auto && <Tag color="blue">{t('tunnel.auto')}</Tag>}
          {row.reverse && <Tag>{t('tunnel.reverse')}</Tag>}
        </Space>
      ),
    },
    {
      title: t('tunnel.portsTitle'),
      key: 'ports',
      responsive: ['md'],
      render: (_, row) => (
        <Space size={[4, 4]} wrap>
          {row.ports.slice(0, 4).map((p) => (
            <Tag key={`${p.listen}/${p.udp}`}>
              {p.listen}
              {p.target !== p.listen ? `→${p.target}` : ''}
              {p.udp ? '/udp' : ''}
            </Tag>
          ))}
          {row.ports.length > 4 && <Tag>+{row.ports.length - 4}</Tag>}
        </Space>
      ),
    },
    {
      title: t('tunnel.state.title'),
      key: 'state',
      render: (_, row) => {
        const failing = [row.iran, row.kharej].find((e) => e?.lastError);
        const tag = <Tag color={stateColor(row.state)}>{t(`tunnel.state.${row.state}`)}</Tag>;
        return failing && row.state !== 'up' ? (
          <Tooltip title={failing.lastError}>{tag}</Tooltip>
        ) : (
          tag
        );
      },
    },
    {
      title: t('tunnel.latency'),
      key: 'rtt',
      responsive: ['md'],
      render: (_, row) => {
        const rtt = row.iran?.rttMs || row.kharej?.rttMs || 0;
        return rtt > 0 ? `${Math.round(rtt)} ms` : '-';
      },
    },
    {
      title: '',
      key: 'actions',
      align: 'end',
      render: (_, row) => (
        <Space size={0}>
          <Tooltip title={t('tunnel.details')}>
            <Button
              type="text"
              icon={<EyeOutlined />}
              aria-label={t('tunnel.details')}
              onClick={() => onOpen(row)}
            />
          </Tooltip>
          <Tooltip title={t('tunnel.restart')}>
            <Button
              type="text"
              icon={<ReloadOutlined />}
              aria-label={t('tunnel.restart')}
              onClick={() => onRestart(row)}
            />
          </Tooltip>
          <Popconfirm
            title={t('tunnel.deleteConfirm', { name: row.name })}
            okText={t('delete')}
            cancelText={t('cancel')}
            okButtonProps={{ danger: true }}
            onConfirm={() => onDelete(row)}
          >
            <Button type="text" danger icon={<DeleteOutlined />} aria-label={t('delete')} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Table
      rowKey="pairUuid"
      size="middle"
      loading={loading && tunnels.length === 0}
      columns={columns}
      dataSource={tunnels}
      pagination={false}
      scroll={{ x: 'max-content' }}
      locale={{
        emptyText: (
          <Empty description={t('tunnel.empty')}>
            <Typography.Text type="secondary">{t('tunnel.emptyHint')}</Typography.Text>
          </Empty>
        ),
      }}
    />
  );
}
