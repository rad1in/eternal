import { useTranslation } from 'react-i18next';
import { Button, Checkbox, InputNumber, Space, Tooltip } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

import type { TunnelPortValues } from '@/schemas/tunnel';

interface PortsEditorProps {
  value?: TunnelPortValues[];
  onChange?: (next: TunnelPortValues[]) => void;
  disabled?: boolean;
}

export const emptyPort = (): TunnelPortValues => ({
  listen: 0,
  target: 0,
  udp: false,
  rateMBps: 0,
  quotaGB: 0,
});

// Value/onChange adapter so FormField can bind a list of port mappings.
export default function PortsEditor({ value = [], onChange, disabled }: PortsEditorProps) {
  const { t } = useTranslation();
  const set = (i: number, patch: Partial<TunnelPortValues>) =>
    onChange?.(value.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  const num = (v: number | null) => v ?? 0;

  return (
    <div className="tunnel-ports">
      <div className="tunnel-ports-head">
        <span>{t('tunnel.ports.listen')}</span>
        <span>{t('tunnel.ports.target')}</span>
        <span>UDP</span>
        <span>{t('tunnel.ports.rate')}</span>
        <span>{t('tunnel.ports.quota')}</span>
        <span />
      </div>
      {value.map((p, i) => (
        <div className="tunnel-ports-row" key={i}>
          <InputNumber
            aria-label={t('tunnel.ports.listen')}
            min={1}
            max={65535}
            precision={0}
            value={p.listen || null}
            disabled={disabled}
            onChange={(v) => set(i, { listen: num(v), target: p.target || num(v) })}
          />
          <InputNumber
            aria-label={t('tunnel.ports.target')}
            min={1}
            max={65535}
            precision={0}
            value={p.target || null}
            disabled={disabled}
            onChange={(v) => set(i, { target: num(v) })}
          />
          <Checkbox
            aria-label="UDP"
            checked={p.udp}
            disabled={disabled}
            onChange={(e) => set(i, { udp: e.target.checked })}
          />
          <Tooltip title={t('tunnel.ports.rateHint')}>
            <InputNumber
              aria-label={t('tunnel.ports.rate')}
              min={0}
              value={p.rateMBps}
              disabled={disabled}
              onChange={(v) => set(i, { rateMBps: num(v) })}
            />
          </Tooltip>
          <Tooltip title={t('tunnel.ports.quotaHint')}>
            <InputNumber
              aria-label={t('tunnel.ports.quota')}
              min={0}
              value={p.quotaGB}
              disabled={disabled}
              onChange={(v) => set(i, { quotaGB: num(v) })}
            />
          </Tooltip>
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            aria-label={t('delete')}
            disabled={disabled || value.length <= 1}
            onClick={() => onChange?.(value.filter((_, j) => j !== i))}
          />
        </div>
      ))}
      <Space>
        <Button
          icon={<PlusOutlined />}
          disabled={disabled}
          onClick={() => onChange?.([...value, emptyPort()])}
        >
          {t('tunnel.ports.add')}
        </Button>
      </Space>
    </div>
  );
}
