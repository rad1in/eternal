import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';

import { LanguageManager } from '@/utils';
import type { LanguageScope } from '@/utils';
import enUS from '../../../internal/web/translation/en-US.json';

const FALLBACK = 'en-US';

const lazyModules = import.meta.glob([
  '../../../internal/web/translation/*.json',
  '!../../../internal/web/translation/en-US.json',
]);

function moduleKeyFor(code: string): string {
  return `../../../internal/web/translation/${code}.json`;
}

export async function readyI18n(scope: LanguageScope = 'panel') {
  let active = LanguageManager.getLanguage(scope);
  if (
    active !== FALLBACK &&
    !Object.prototype.hasOwnProperty.call(lazyModules, moduleKeyFor(active))
  ) {
    active = FALLBACK;
  }

  await i18next.use(initReactI18next).init({
    lng: active,
    fallbackLng: FALLBACK,
    resources: { [FALLBACK]: { translation: enUS } },
    interpolation: { escapeValue: false, prefix: '{', suffix: '}' },
    returnNull: false,
  });
  if (active !== FALLBACK) {
    const loader = lazyModules[moduleKeyFor(active)] as
      | (() => Promise<{ default: Record<string, unknown> }>)
      | undefined;
    if (loader) {
      const mod = await loader();
      const messages = (mod.default ?? mod) as Record<string, unknown>;
      i18next.addResourceBundle(active, 'translation', messages, true, true);
      await i18next.changeLanguage(active);
    }
  }
  return i18next;
}

export async function switchLanguage(code: string) {
  const next = Object.prototype.hasOwnProperty.call(lazyModules, moduleKeyFor(code))
    ? code
    : FALLBACK;
  if (next !== FALLBACK && !i18next.hasResourceBundle(next, 'translation')) {
    const loader = lazyModules[moduleKeyFor(next)] as () => Promise<{
      default: Record<string, unknown>;
    }>;
    const mod = await loader();
    i18next.addResourceBundle(next, 'translation', mod.default ?? mod, true, true);
  }
  await i18next.changeLanguage(next);
}

export { i18next as i18n };
