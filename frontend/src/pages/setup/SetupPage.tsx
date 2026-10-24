import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, ConfigProvider, Form, Grid, Result, Spin, Steps } from 'antd';

import { switchLanguage } from '@/i18n/react';
import {
  createSetupApi,
  tokenFromLocation,
  type SetupCode,
  type SetupFinished,
  type SetupStatus,
} from '@/lib/setup/setupApi';
import {
  normalizeBasePath,
  validateBasePath,
  validatePassword,
  validatePort,
  validateUsername,
} from '@/lib/setup/validation';
import { CookieManager, LanguageManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import BrandMark from '@/components/ui/BrandMark';
import AccountStep from './AccountStep';
import AddressStep from './AddressStep';
import DoneState from './DoneState';
import ReviewStep from './ReviewStep';
import WelcomeStep from './WelcomeStep';
import type { FieldErrors, FieldName, PortStatus, SetupFormState } from './types';
import './SetupPage.css';

const RTL_LANGUAGES = new Set(['fa-IR', 'ar-EG']);
const PORT_CHECK_DEBOUNCE_MS = 400;
const STEP_KEYS = [
  'setup.steps.welcome',
  'setup.steps.account',
  'setup.steps.address',
  'setup.steps.review',
] as const;
const FIELD_STEP: Record<FieldName, number> = { username: 1, password: 1, port: 2, basePath: 2 };

type Boot =
  | { phase: 'loading' }
  | { phase: 'ready'; status: SetupStatus }
  | { phase: 'fatal'; code: SetupCode };

function currentLanguage(): string {
  const cookie = CookieManager.getCookie('lang');
  return LanguageManager.isSupportLanguage(cookie) ? cookie : 'en-US';
}

export default function SetupPage() {
  const { t, i18n } = useTranslation();
  const { antdThemeConfig } = useTheme();
  const screens = Grid.useBreakpoint();
  const token = useMemo(() => tokenFromLocation(), []);
  const api = useMemo(() => createSetupApi(token), [token]);

  const [boot, setBoot] = useState<Boot>(
    token ? { phase: 'loading' } : { phase: 'fatal', code: 'bad_token' },
  );
  const [attempt, setAttempt] = useState(0);
  const [step, setStep] = useState(0);
  const [form, setForm] = useState<SetupFormState>({
    username: '',
    password: '',
    confirm: '',
    port: null,
    basePath: '',
    language: currentLanguage(),
  });
  const [serverErrors, setServerErrors] = useState<FieldErrors>({});
  const [portCheck, setPortCheck] = useState<{ port: number; status: PortStatus } | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [failure, setFailure] = useState<SetupCode | null>(null);
  const [done, setDone] = useState<SetupFinished | null>(null);

  const rtl = RTL_LANGUAGES.has(form.language);

  useEffect(() => {
    document.documentElement.dir = rtl ? 'rtl' : 'ltr';
    document.documentElement.lang = form.language;
  }, [rtl, form.language]);

  useEffect(() => {
    if (!token) return undefined;
    let cancelled = false;
    void api.status().then((res) => {
      if (cancelled) return;
      if (!res.ok) {
        setBoot({ phase: 'fatal', code: res.code });
      } else if (!res.data.pending) {
        setBoot({ phase: 'fatal', code: 'already_done' });
      } else {
        setForm((f) => ({
          ...f,
          port: f.port ?? res.data.suggestedPort,
          basePath: f.basePath || res.data.suggestedBasePath,
        }));
        setBoot({ phase: 'ready', status: res.data });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [api, token, attempt]);

  const retry = () => {
    setBoot({ phase: 'loading' });
    setAttempt((n) => n + 1);
  };

  const change = useCallback((patch: Partial<SetupFormState>) => {
    setForm((f) => ({ ...f, ...patch }));
    setServerErrors((prev) => {
      const next = { ...prev };
      if ('username' in patch) delete next.username;
      if ('password' in patch) delete next.password;
      if ('port' in patch) delete next.port;
      if ('basePath' in patch) delete next.basePath;
      return next;
    });
  }, []);

  const changeLanguage = useCallback((code: string) => {
    CookieManager.setCookie('lang', code, 365);
    setForm((f) => ({ ...f, language: code }));
    void switchLanguage(code);
  }, []);

  // Debounced so typing a port does not fire a probe per keystroke.
  const port = form.port;
  useEffect(() => {
    if (boot.phase !== 'ready' || port === null || validatePort(port) !== null) return undefined;
    let stale = false;
    const timer = window.setTimeout(() => {
      void api.checkPort(port).then((res) => {
        if (stale) return;
        setPortCheck({ port, status: res.ok ? (res.data.free ? 'free' : 'used') : 'unknown' });
      });
    }, PORT_CHECK_DEBOUNCE_MS);
    return () => {
      stale = true;
      window.clearTimeout(timer);
    };
  }, [api, boot.phase, port]);

  const portStatus: PortStatus =
    port === null || validatePort(port) !== null
      ? 'idle'
      : portCheck?.port === port
        ? portCheck.status
        : 'checking';

  const stepValid = (index: number): boolean => {
    if (index === 1) {
      return (
        validateUsername(form.username) === null &&
        validatePassword(form.password, form.username) === null &&
        form.confirm === form.password &&
        !serverErrors.username &&
        !serverErrors.password
      );
    }
    if (index === 2) {
      return (
        port !== null &&
        validatePort(port) === null &&
        validateBasePath(form.basePath) === null &&
        portStatus !== 'checking' &&
        portStatus !== 'used' &&
        !serverErrors.port &&
        !serverErrors.basePath
      );
    }
    return true;
  };

  const finish = async () => {
    if (port === null) return;
    setSubmitting(true);
    setFailure(null);
    const basePath = normalizeBasePath(form.basePath);
    const res = await api.finish({
      username: form.username,
      password: form.password,
      port,
      basePath,
      language: form.language,
    });
    setSubmitting(false);
    if (res.ok) {
      setDone(res.data);
      return;
    }
    switch (res.code) {
      case 'bad_token':
      case 'already_done':
        setBoot({ phase: 'fatal', code: res.code });
        break;
      case 'username_invalid':
      case 'password_weak':
      case 'base_path_invalid':
      case 'port_invalid': {
        const field: FieldName =
          res.code === 'username_invalid'
            ? 'username'
            : res.code === 'password_weak'
              ? 'password'
              : res.code === 'port_invalid'
                ? 'port'
                : 'basePath';
        setServerErrors({ [field]: res.code });
        setStep(FIELD_STEP[field]);
        break;
      }
      case 'port_in_use':
        setPortCheck({ port, status: 'used' });
        setStep(FIELD_STEP.port);
        break;
      default:
        setFailure(res.code);
    }
  };

  const onSubmit = (e?: FormEvent) => {
    e?.preventDefault();
    if (submitting || !stepValid(step)) return;
    if (step < STEP_KEYS.length - 1) setStep(step + 1);
    else void finish();
  };

  const dir = rtl ? 'rtl' : 'ltr';
  const wide = screens.md !== false;

  let body;
  if (done) {
    body = <DoneState port={done.port} basePath={done.basePath} />;
  } else if (boot.phase === 'loading') {
    body = (
      <div className="setup-center">
        <Spin size="large" />
      </div>
    );
  } else if (boot.phase === 'fatal') {
    const terminal = boot.code === 'bad_token' || boot.code === 'already_done';
    body = (
      <Result
        status={boot.code === 'already_done' ? 'info' : 'warning'}
        title={
          boot.code === 'bad_token'
            ? t('setup.fatal.badTokenTitle')
            : boot.code === 'already_done'
              ? t('setup.fatal.alreadyDoneTitle')
              : t('setup.title')
        }
        subTitle={t(`setup.errors.${boot.code}`)}
        extra={
          terminal ? undefined : (
            <Button type="primary" onClick={retry}>
              {t('setup.retry')}
            </Button>
          )
        }
      />
    );
  } else {
    const { status } = boot;
    const last = step === STEP_KEYS.length - 1;
    body = (
      <>
        <Steps
          size="small"
          current={step}
          orientation={wide ? 'horizontal' : 'vertical'}
          items={STEP_KEYS.map((key) => ({ title: t(key) }))}
          className="setup-steps"
        />
        <Form layout="vertical" onSubmitCapture={onSubmit} className="setup-form" noValidate>
          {step === 0 && (
            <WelcomeStep
              version={status.version}
              language={form.language}
              onLanguage={changeLanguage}
            />
          )}
          {step === 1 && <AccountStep form={form} serverErrors={serverErrors} onChange={change} />}
          {step === 2 && (
            <AddressStep
              form={form}
              serverHost={status.serverHost}
              serverErrors={serverErrors}
              portStatus={portStatus}
              onChange={change}
            />
          )}
          {step === 3 && (
            <ReviewStep
              form={form}
              serverHost={status.serverHost}
              failure={failure ? t(`setup.errors.${failure}`) : null}
            />
          )}
          <div className="setup-actions">
            <Button disabled={step === 0 || submitting} onClick={() => setStep(step - 1)}>
              {t('setup.nav.back')}
            </Button>
            <Button
              type="primary"
              htmlType="submit"
              loading={submitting}
              disabled={!stepValid(step)}
            >
              {last ? t('setup.review.create') : t('setup.nav.next')}
            </Button>
          </div>
        </Form>
      </>
    );
  }

  return (
    <ConfigProvider theme={antdThemeConfig} direction={dir}>
      <main className="setup-page" dir={dir} lang={i18n.language}>
        <Card className="setup-card">
          <header className="setup-header">
            <BrandMark className="setup-brand" />
            <span className="setup-title">{t('setup.title')}</span>
          </header>
          {body}
        </Card>
      </main>
    </ConfigProvider>
  );
}
