import { useTranslation } from 'react-i18next';
import { Button, Card, List, Progress, Space, Tag, Typography } from 'antd';

import type { ShopService } from '@/generated/zod';

const BYTES_PER_GB = 1024 ** 3;

interface ServicesListProps {
  services: ShopService[];
  onRenew: (email: string) => void;
  onAddon?: (email: string) => void;
}

function gb(bytes: number): string {
  return (Math.round((bytes / BYTES_PER_GB) * 100) / 100).toString();
}

export default function ServicesList({ services, onRenew, onAddon }: ServicesListProps) {
  const { t } = useTranslation();
  return (
    <Card size="small" title={t('shopbot.app.services')}>
      <List
        size="small"
        locale={{ emptyText: t('shopbot.app.noServices') }}
        dataSource={services}
        renderItem={(s) => {
          const used = s.up + s.down;
          const percent = s.total > 0 ? Math.min(100, Math.round((used * 100) / s.total)) : 0;
          return (
            <List.Item>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space wrap>
                  <Typography.Text strong dir="ltr">
                    {s.email}
                  </Typography.Text>
                  {!s.enable && <Tag color="red">{t('shopbot.app.disabled')}</Tag>}
                </Space>
                <Progress percent={percent} size="small" showInfo={s.total > 0} />
                <Typography.Text type="secondary">
                  {s.total > 0
                    ? t('shopbot.app.usage', {
                        used: gb(used),
                        total: gb(s.total),
                      })
                    : t('shop.unlimitedData')}
                  {' · '}
                  {s.expiryTime > 0
                    ? t('shopbot.app.expiresOn', {
                        date: new Date(s.expiryTime).toLocaleDateString(),
                      })
                    : t('shop.noExpiry')}
                </Typography.Text>
                <Space wrap>
                  <Button size="small" type="primary" onClick={() => onRenew(s.email)}>
                    {t('shopbot.app.renew')}
                  </Button>
                  {onAddon && (
                    <Button size="small" onClick={() => onAddon(s.email)}>
                      {t('shopbot.app.addon')}
                    </Button>
                  )}
                  {s.subscriptionUrl && (
                    <Typography.Text copyable={{ text: s.subscriptionUrl }}>
                      {t('shopbot.app.copyLink')}
                    </Typography.Text>
                  )}
                </Space>
              </Space>
            </List.Item>
          );
        }}
      />
    </Card>
  );
}
