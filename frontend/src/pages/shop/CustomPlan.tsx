import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, InputNumber, Space, Typography } from 'antd';

import type { ShopCustom } from '@/generated/zod';
import { customPrice, customSizeAllowed } from '@/lib/customOffer';
import { formatMoney } from '@/lib/money';
import type { Offer } from './offer';

interface CustomPlanProps {
  custom: ShopCustom;
  // An add-on may add just data or just days.
  addon: boolean;
  onSelect: (offer: Offer) => void;
}

export default function CustomPlan({ custom, addon, onSelect }: CustomPlanProps) {
  const { t } = useTranslation();
  const [gb, setGb] = useState(addon ? 0 : custom.minGb);
  const [days, setDays] = useState(addon ? 0 : custom.minDays);
  const kind = addon ? 'addon' : 'new';
  const valid = customSizeAllowed(custom, kind, gb, days);
  const price = customPrice(custom, gb, days);

  return (
    <Card size="small" title={t('shopbot.app.custom')}>
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Space wrap>
          <InputNumber
            aria-label={t('shopbot.app.customGb')}
            addonAfter="GB"
            min={0}
            max={custom.maxGb}
            precision={0}
            value={gb}
            onChange={(v) => setGb(v ?? 0)}
          />
          <InputNumber
            aria-label={t('shopbot.app.customDays')}
            addonAfter={t('shopbot.app.customDays')}
            min={0}
            max={custom.maxDays}
            precision={0}
            value={days}
            onChange={(v) => setDays(v ?? 0)}
          />
        </Space>
        <Typography.Text type="secondary">
          {t('shopbot.app.customBounds', {
            minGb: custom.minGb,
            maxGb: custom.maxGb,
            minDays: custom.minDays,
            maxDays: custom.maxDays,
          })}
        </Typography.Text>
        <Typography.Text strong>
          {t('shopbot.app.customTotal')}: {formatMoney(price, custom.currency)}
        </Typography.Text>
        <Button
          type="primary"
          disabled={!valid}
          onClick={() =>
            onSelect({
              planId: 0,
              name: `${gb} GB / ${days} ${t('shopbot.app.customDays')}`,
              price,
              currency: custom.currency,
              custom: { gb, days },
            })
          }
        >
          {t('shopbot.app.continue')}
        </Button>
      </Space>
    </Card>
  );
}
