import type { SalesConfig, SalesConfigView } from '@/generated/zod';
import { fromMinor, toMinor, type Currency } from '@/lib/money';
import type { SalesConfigValues } from '@/schemas/sales';

type JoinChat = SalesConfig['bot']['forceJoin'][number];
type MenuLink = SalesConfig['bot']['links'][number];

const lines = (text: string): string[] =>
  text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '');

// One required channel per line: "@channel" or "-100123…", optionally followed
// by " | join link".
export function parseJoinLines(text: string): JoinChat[] {
  return lines(text).map((line) => {
    const [chat, ...rest] = line.split('|');
    return { chat: chat.trim(), link: rest.join('|').trim() };
  });
}

export function joinToLines(list: JoinChat[]): string {
  return list.map((j) => (j.link ? `${j.chat} | ${j.link}` : j.chat)).join('\n');
}

// One menu button per line: "label | https://…". Only the first pipe splits.
export function parseLinkLines(text: string): MenuLink[] {
  return lines(text).flatMap((line) => {
    const at = line.indexOf('|');
    if (at < 0) return [];
    const label = line.slice(0, at).trim();
    const url = line.slice(at + 1).trim();
    return label && url ? [{ label, url }] : [];
  });
}

export function linksToLines(list: MenuLink[]): string {
  return list.map((l) => `${l.label} | ${l.url}`).join('\n');
}

export function configToForm(c: SalesConfigView): SalesConfigValues {
  const cur = c.custom.currency || 'IRT';
  return {
    ...c,
    clearSecrets: [],
    bot: {
      miniApp: c.bot.miniApp,
      receiptChatId: c.bot.receiptChatId,
      forceJoinText: joinToLines(c.bot.forceJoin ?? []),
      linksText: linksToLines(c.bot.links ?? []),
    },
    custom: {
      ...c.custom,
      currency: cur as Currency,
      pricePerGb: fromMinor(c.custom.pricePerGb, cur),
      pricePerDay: fromMinor(c.custom.pricePerDay, cur),
    },
  };
}

export function formToConfig(v: SalesConfigValues): SalesConfig {
  const { bot, custom, ...rest } = v;
  return {
    ...rest,
    bot: {
      miniApp: bot.miniApp,
      receiptChatId: bot.receiptChatId,
      forceJoin: parseJoinLines(bot.forceJoinText),
      links: parseLinkLines(bot.linksText),
    },
    custom: {
      ...custom,
      pricePerGb: toMinor(custom.pricePerGb, custom.currency),
      pricePerDay: toMinor(custom.pricePerDay, custom.currency),
    },
  };
}
