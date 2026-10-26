// The address this panel is reached on, base path included, as a server without a panel
// should call it.
export function panelOrigin(): string {
  const base = (window.X_UI_BASE_PATH ?? '/').replace(/\/+$/, '');
  return `${window.location.origin}${base}`;
}
