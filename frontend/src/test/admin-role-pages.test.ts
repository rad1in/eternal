import { expect, test } from 'vitest';

import { canOpenPage } from '@/lib/adminRole';

// The server answers 403 to every role-denied route; the menu must not offer the page.
test.each([
  ['owner', true],
  ['admin', true],
  ['viewer', true],
  ['operator', false],
])('%s opening /tunnels: %s', (role, allowed) => {
  expect(canOpenPage(role, '/tunnels')).toBe(allowed);
});
