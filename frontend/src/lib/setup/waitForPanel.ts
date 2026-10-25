export interface WaitForPanelOptions {
  intervalMs?: number;
  timeoutMs?: number;
  signal?: AbortSignal;
}

// no-cors keeps the probe working across origins; any response at all means the listener is up.
export async function waitForPanel(
  url: string,
  { intervalMs = 1500, timeoutMs = 40000, signal }: WaitForPanelOptions = {},
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (!signal?.aborted && Date.now() < deadline) {
    try {
      await fetch(url, { mode: 'no-cors', cache: 'no-store' });
      return true;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, intervalMs));
    }
  }
  return false;
}
