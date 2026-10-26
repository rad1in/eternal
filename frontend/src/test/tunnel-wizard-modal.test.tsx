import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';

import TunnelWizard from '@/pages/tunnels/TunnelWizard';
import type { NodeRecord } from '@/schemas/node';

import { renderWithProviders } from './test-utils';

const nodes = [{ id: 2, name: 'de-fra', enable: true } as NodeRecord];

function mount(onCreate = vi.fn().mockResolvedValue(null)) {
  renderWithProviders(
    <TunnelWizard
      open
      nodes={nodes}
      capabilities={{
        binaryAvailable: true,
        platform: 'linux',
        publicIPv4: '203.0.113.7',
        carriers: ['tcp', 'udp'],
        canSeeToken: true,
      }}
      creating={false}
      onCreate={onCreate}
      onClose={() => {}}
    />,
  );
  return onCreate;
}

const next = () => fireEvent.click(screen.getByRole('button', { name: 'Next' }));

async function pickNode() {
  const dialog = screen.getByRole('dialog');
  fireEvent.mouseDown(within(dialog).getByRole('combobox'));
  fireEvent.click(await screen.findByText('de-fra'));
}

describe('TunnelWizard', () => {
  it('stays on the first step and says why when the name is empty', async () => {
    mount();
    next();
    expect(await screen.findByText(/Use 1-63 letters/)).toBeTruthy();
    expect(screen.queryByText('Listen port')).toBeNull();
  });

  it('creates a node-to-panel tunnel with the default port', async () => {
    const onCreate = mount();
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'de-ir' } });
    await pickNode();
    next();
    expect(await screen.findByText('Listen port')).toBeTruthy();
    next();
    expect(await screen.findByText('Advanced')).toBeTruthy();
    next();
    expect(
      await screen.findByText('Review', {
        selector: '.ant-descriptions-item-label,.ant-steps-item-title',
      }),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1));
    const sent = onCreate.mock.calls[0][0];
    expect(sent).toMatchObject({
      name: 'de-ir',
      kharejKind: 'node',
      kharejNodeId: 2,
      iranKind: 'local',
      carrier: 'auto',
      ports: [{ listen: 443, target: 443, udp: false }],
    });
  });

  it('gives the setup command for a Kharej server without a panel and waits for its token', async () => {
    const onCreate = mount();
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'x-ir' } });
    fireEvent.click(screen.getAllByText('Server without a panel')[0]);
    next();
    await screen.findByText('Listen port');
    next();
    await screen.findByText('Advanced');
    next();
    const command = (await screen.findByDisplayValue(/--role kharej/)) as HTMLTextAreaElement;
    expect(command.value).toContain('/install-tunnel.sh');
    expect(command.value).toContain('--ports 443');

    fireEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByText(/Paste the token that starts with wat1:/)).toBeTruthy();
    expect(onCreate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText('Token from the Kharej server'), {
      target: { value: 'wat1:abc' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1));
    expect(onCreate.mock.calls[0][0]).toMatchObject({ kharejKind: 'external', token: 'wat1:abc' });
  });
});
