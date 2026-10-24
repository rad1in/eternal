import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Result, Space, Typography, message } from 'antd';
import { CopyOutlined, LoadingOutlined } from '@ant-design/icons';

import { waitForPanel } from '@/lib/setup/waitForPanel';
import { ClipboardManager } from '@/utils';

interface Props {
  port: number;
  basePath: string;
}

export function finalPanelUrl(port: number, basePath: string): string {
  const { protocol, hostname } = window.location;
  const host = hostname.includes(':') ? `[${hostname}]` : hostname;
  return `${protocol}//${host}:${port}${basePath}`;
}

export default function DoneState({ port, basePath }: Props) {
  const { t } = useTranslation();
  const url = finalPanelUrl(port, basePath);
  const [state, setState] = useState<'waiting' | 'timeout'>('waiting');
  const [messageApi, messageHolder] = message.useMessage();

  useEffect(() => {
    const abort = new AbortController();
    void waitForPanel(url, { signal: abort.signal }).then((up) => {
      if (abort.signal.aborted) return;
      if (up) window.location.assign(url);
      else setState('timeout');
    });
    return () => abort.abort();
  }, [url]);

  const copy = async () => {
    if (await ClipboardManager.copyText(url)) messageApi.success(t('setup.done.copied'));
  };

  return (
    <Result
      status="success"
      title={t('setup.done.title')}
      subTitle={t('setup.done.desc')}
      extra={
        <Space orientation="vertical" size="middle" className="setup-done">
          {messageHolder}
          <Typography.Text type="secondary">{t('setup.done.address')}</Typography.Text>
          <Space.Compact>
            <Button
              type="link"
              href={url}
              className="setup-done-link"
              data-testid="setup-final-url"
            >
              <span dir="ltr">{url}</span>
            </Button>
            <Button icon={<CopyOutlined />} onClick={copy}>
              {t('setup.done.copy')}
            </Button>
          </Space.Compact>
          <div role="status" aria-live="polite">
            {state === 'waiting' ? (
              <Typography.Text type="secondary">
                <LoadingOutlined /> {t('setup.done.waiting')}
              </Typography.Text>
            ) : (
              <Typography.Text type="warning">{t('setup.done.timeout')}</Typography.Text>
            )}
          </div>
          {state === 'timeout' && (
            <Button type="primary" href={url}>
              {t('setup.done.open')}
            </Button>
          )}
        </Space>
      }
    />
  );
}
