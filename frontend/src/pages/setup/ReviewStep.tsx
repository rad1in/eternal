import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Descriptions, Typography } from 'antd';
import { EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';

import { LanguageManager } from '@/utils';
import { previewUrl } from '@/lib/setup/validation';
import type { SetupFormState } from './types';

interface Props {
  form: SetupFormState;
  serverHost: string;
  failure: string | null;
}

export default function ReviewStep({ form, serverHost, failure }: Props) {
  const { t } = useTranslation();
  const [reveal, setReveal] = useState(false);
  const language =
    LanguageManager.supportedLanguages.find((l) => l.value === form.language)?.name ??
    form.language;

  return (
    <div className="setup-step">
      <Typography.Title level={4} className="setup-step-title">
        {t('setup.review.heading')}
      </Typography.Title>
      <Typography.Paragraph type="secondary">{t('setup.review.intro')}</Typography.Paragraph>
      <Descriptions column={1} bordered size="small" className="setup-summary">
        <Descriptions.Item label={t('setup.review.username')}>
          <code dir="ltr">{form.username}</code>
        </Descriptions.Item>
        <Descriptions.Item label={t('setup.review.password')}>
          <span className="setup-secret">
            <code dir="ltr" data-testid="setup-review-password">
              {reveal ? form.password : '•'.repeat(Math.min(form.password.length, 12))}
            </code>
            <Button
              size="small"
              type="text"
              icon={reveal ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              aria-label={reveal ? t('setup.account.hide') : t('setup.account.show')}
              aria-pressed={reveal}
              onClick={() => setReveal((v) => !v)}
            />
          </span>
        </Descriptions.Item>
        <Descriptions.Item label={t('setup.review.url')}>
          <code dir="ltr" data-testid="setup-review-url">
            {previewUrl(serverHost, form.port ?? 0, form.basePath)}
          </code>
        </Descriptions.Item>
        <Descriptions.Item label={t('setup.review.language')}>{language}</Descriptions.Item>
      </Descriptions>
      <Alert
        type="warning"
        showIcon
        title={t('setup.review.saveTitle')}
        description={t('setup.review.saveWarning')}
      />
      {failure && <Alert type="error" showIcon title={failure} role="alert" />}
    </div>
  );
}
