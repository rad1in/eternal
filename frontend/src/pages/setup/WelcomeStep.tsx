import { useTranslation } from 'react-i18next';
import { Select, Typography } from 'antd';

import { LanguageManager } from '@/utils';

interface Props {
  version: string;
  language: string;
  onLanguage: (code: string) => void;
}

const OPTIONS = LanguageManager.supportedLanguages.map((l) => ({
  value: l.value,
  label: (
    <span>
      <span aria-hidden="true">{l.icon}</span> {l.name}
    </span>
  ),
}));

export default function WelcomeStep({ version, language, onLanguage }: Props) {
  const { t } = useTranslation();
  return (
    <div className="setup-step">
      <Typography.Title level={4} className="setup-step-title">
        {t('setup.welcome.heading')}
      </Typography.Title>
      <Typography.Paragraph type="secondary">{t('setup.welcome.intro')}</Typography.Paragraph>
      {version && (
        <Typography.Text type="secondary" className="setup-version">
          {t('setup.welcome.version', { version })}
        </Typography.Text>
      )}
      <div className="setup-field">
        <label htmlFor="setup-language" className="setup-label">
          {t('setup.welcome.language')}
        </label>
        <Select
          id="setup-language"
          value={language}
          options={OPTIONS}
          onChange={onLanguage}
          className="setup-language"
          showSearch={{ optionFilterProp: 'value' }}
        />
      </div>
      <Typography.Text strong>{t('setup.welcome.nextTitle')}</Typography.Text>
      <ol className="setup-next">
        <li>{t('setup.welcome.next1')}</li>
        <li>{t('setup.welcome.next2')}</li>
        <li>{t('setup.welcome.next3')}</li>
      </ol>
    </div>
  );
}
