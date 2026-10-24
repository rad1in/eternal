import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Space, Typography, message } from 'antd';
import {
  CopyOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import {
  generatePassword,
  passwordStrength,
  validatePassword,
  validateUsername,
} from '@/lib/setup/validation';
import { ClipboardManager } from '@/utils';
import type { FieldErrors, SetupFormState } from './types';

interface Props {
  form: SetupFormState;
  serverErrors: FieldErrors;
  onChange: (patch: Partial<SetupFormState>) => void;
}

const STRENGTH_LABELS = [
  'setup.account.strength0',
  'setup.account.strength1',
  'setup.account.strength2',
  'setup.account.strength3',
  'setup.account.strength4',
] as const;

export default function AccountStep({ form, serverErrors, onChange }: Props) {
  const { t } = useTranslation();
  const [reveal, setReveal] = useState(false);
  const [touched, setTouched] = useState({ username: false, password: false, confirm: false });
  const [messageApi, messageHolder] = message.useMessage();

  const usernameCode =
    serverErrors.username ?? (touched.username ? validateUsername(form.username) : null);
  const passwordCode =
    serverErrors.password ??
    (touched.password ? validatePassword(form.password, form.username) : null);
  const mismatch = touched.confirm && form.confirm !== form.password;
  const strength = passwordStrength(form.password, form.username);

  const generate = () => {
    const pw = generatePassword();
    onChange({ password: pw, confirm: pw });
    setTouched((s) => ({ ...s, password: true, confirm: true }));
    setReveal(true);
  };
  const copy = async () => {
    if (await ClipboardManager.copyText(form.password)) {
      messageApi.success(t('setup.account.copied'));
    }
  };

  return (
    <div className="setup-step">
      {messageHolder}
      <Typography.Title level={4} className="setup-step-title">
        {t('setup.account.heading')}
      </Typography.Title>
      <Typography.Paragraph type="secondary">{t('setup.account.intro')}</Typography.Paragraph>

      <Form.Item
        label={t('setup.account.username')}
        htmlFor="setup-username"
        validateStatus={usernameCode ? 'error' : undefined}
        help={usernameCode ? t(`setup.errors.${usernameCode}`) : t('setup.account.usernameHelp')}
        required
      >
        <Input
          id="setup-username"
          autoFocus
          autoComplete="username"
          dir="ltr"
          maxLength={64}
          value={form.username}
          onChange={(e) => onChange({ username: e.target.value })}
          onBlur={() => setTouched((s) => ({ ...s, username: true }))}
        />
      </Form.Item>

      <Form.Item
        label={t('setup.account.password')}
        htmlFor="setup-password"
        validateStatus={passwordCode ? 'error' : undefined}
        help={passwordCode ? t(`setup.errors.${passwordCode}`) : t('setup.account.passwordHelp')}
        required
      >
        <Space.Compact block>
          <Input
            id="setup-password"
            type={reveal ? 'text' : 'password'}
            autoComplete="new-password"
            dir="ltr"
            value={form.password}
            onChange={(e) => onChange({ password: e.target.value })}
            onBlur={() => setTouched((s) => ({ ...s, password: true }))}
          />
          <Button
            icon={reveal ? <EyeInvisibleOutlined /> : <EyeOutlined />}
            aria-label={reveal ? t('setup.account.hide') : t('setup.account.show')}
            aria-pressed={reveal}
            onClick={() => setReveal((v) => !v)}
          />
          <Button
            icon={<CopyOutlined />}
            aria-label={t('setup.account.copy')}
            disabled={!form.password}
            onClick={copy}
          />
        </Space.Compact>
      </Form.Item>

      <div className="setup-strength">
        <span className="setup-strength-label">{t('setup.account.strength')}</span>
        <div
          className={`setup-meter level-${strength}`}
          role="meter"
          aria-valuemin={0}
          aria-valuemax={4}
          aria-valuenow={strength}
          aria-valuetext={form.password ? t(STRENGTH_LABELS[strength]) : undefined}
          aria-label={t('setup.account.strength')}
        >
          {[1, 2, 3, 4].map((i) => (
            <span key={i} className={i <= strength ? 'is-on' : undefined} />
          ))}
        </div>
        <span className="setup-strength-value">
          {form.password ? t(STRENGTH_LABELS[strength]) : ''}
        </span>
      </div>

      <Form.Item
        label={t('setup.account.confirm')}
        htmlFor="setup-confirm"
        validateStatus={mismatch ? 'error' : undefined}
        help={mismatch ? t('setup.account.mismatch') : undefined}
        required
      >
        <Input
          id="setup-confirm"
          type={reveal ? 'text' : 'password'}
          autoComplete="new-password"
          dir="ltr"
          value={form.confirm}
          onChange={(e) => onChange({ confirm: e.target.value })}
          onBlur={() => setTouched((s) => ({ ...s, confirm: true }))}
        />
      </Form.Item>

      <Button icon={<ThunderboltOutlined />} onClick={generate}>
        {t('setup.account.generate')}
      </Button>
    </div>
  );
}
