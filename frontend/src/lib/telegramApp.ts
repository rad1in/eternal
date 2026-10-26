// Telegram opens a Mini App with its launch data in the URL fragment, so the
// page needs no Telegram script to learn who opened it.
export function launchInitData(hash: string): string {
  return new URLSearchParams(hash.replace(/^#/, '')).get('tgWebAppData') ?? '';
}
