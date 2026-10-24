import { useTranslation } from 'react-i18next';
import { Alert, Button, Form, Input, InputNumber, Space, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import {
  PORT_MAX,
  PORT_MIN,
  previewUrl,
  randomBasePath,
  randomPort,
  validateBasePath,
  validatePort,
} from '@/lib/setup/validation';
import type { FieldErrors, PortStatus, SetupFormState } from './types';

interface Props {
  form: SetupFormState;
  serverHost: string;
  serverErrors: FieldErrors;
  portStatus: PortStatus;
  onChange: (patch: Partial<SetupFormState>) => void;
}

export default function AddressStep({
  form,
  serverHost,
  serverErrors,
  portStatus,
  onChange,
}: Props) {
  const { t } = useTranslation();

  const portCode =
    serverErrors.port ??
    (form.port === null || validatePort(form.port) !== null
      ? 'port_invalid'
      : portStatus === 'used'
        ? 'port_in_use'
        : null);
  const pathCode = serverErrors.basePath ?? validateBasePath(form.basePath);

  let portHelp: string = t('setup.address.portHelp');
  if (!portCode && portStatus === 'checking') portHelp = t('setup.address.portChecking');
  if (!portCode && portStatus === 'free') portHelp = t('setup.address.portFree');

  const preview =
    form.port !== null && !validatePort(form.port) && !validateBasePath(form.basePath)
      ? previewUrl(serverHost, form.port, form.basePath)
      : '';

  return (
    <div className="setup-step">
      <Typography.Title level={4} className="setup-step-title">
        {t('setup.address.heading')}
      </Typography.Title>
      <Typography.Paragraph type="secondary">{t('setup.address.intro')}</Typography.Paragraph>

      <Form.Item
        label={t('setup.address.port')}
        htmlFor="setup-port"
        validateStatus={portCode ? 'error' : portStatus === 'checking' ? 'validating' : undefined}
        help={portCode ? t(`setup.errors.${portCode}`) : portHelp}
        required
      >
        <Space.Compact block>
          <InputNumber
            id="setup-port"
            autoFocus
            controls={false}
            min={PORT_MIN}
            max={PORT_MAX}
            precision={0}
            value={form.port}
            onChange={(v) => onChange({ port: typeof v === 'number' ? v : null })}
            className="setup-port"
            style={{ width: '100%' }}
          />
          <Button
            icon={<ReloadOutlined />}
            onClick={() => onChange({ port: randomPort() })}
            aria-label={t('setup.address.randomPort')}
          >
            {t('setup.address.random')}
          </Button>
        </Space.Compact>
      </Form.Item>

      <Form.Item
        label={t('setup.address.basePath')}
        htmlFor="setup-base-path"
        validateStatus={pathCode ? 'error' : undefined}
        help={pathCode ? t(`setup.errors.${pathCode}`) : t('setup.address.basePathHelp')}
        required
      >
        <Space.Compact block>
          <Input
            id="setup-base-path"
            dir="ltr"
            autoComplete="off"
            spellCheck={false}
            value={form.basePath}
            onChange={(e) => onChange({ basePath: e.target.value })}
          />
          <Button
            icon={<ReloadOutlined />}
            onClick={() => onChange({ basePath: randomBasePath() })}
            aria-label={t('setup.address.randomPath')}
          >
            {t('setup.address.random')}
          </Button>
        </Space.Compact>
      </Form.Item>

      <div className="setup-preview">
        <span className="setup-label">{t('setup.address.preview')}</span>
        <code dir="ltr" data-testid="setup-preview">
          {preview || '-'}
        </code>
      </div>
      <Alert type="info" showIcon title={t('setup.address.httpsNote')} />
    </div>
  );
}
