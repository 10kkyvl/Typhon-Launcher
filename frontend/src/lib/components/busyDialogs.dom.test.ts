import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { cloudResolveConflict, type CloudConflict } from '../services/cloudsaves';
import { cloudConflictTarget, cloudStatus } from '../stores/cloudsaves';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
}));

// The operation each dialog waits on: held open by the test, then failed.
function held() {
  let fail!: (err: unknown) => void;
  const promise = new Promise<never>((_resolve, reject) => {
    fail = reject;
  });
  return { promise, fail };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  cloudConflictTarget.set(null);
  cloudStatus.set(null);
  toasts.set([]);
  app.remove();
  vi.clearAllMocks();
});

describe('a dialog busy with its operation', () => {
  it('CloudConflictModal: Escape does nothing while the conflict is resolved, and a failure shows in the dialog', async () => {
    const conflict = { localUpdatedAt: '2026-11-01T10:00:00Z', remoteUpdatedAt: '2026-11-02T10:00:00Z', localSize: 10, remoteSize: 20, remoteDevice: 'Deck' } as CloudConflict;
    cloudStatus.set({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }] });
    cloudConflictTarget.set({ gameId: 'g1', title: 'Hades' });
    const resolving = held();
    vi.mocked(cloudResolveConflict).mockReturnValueOnce(resolving.promise);
    render(CloudConflictModal, { target: app });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));

    await keys.keyboard('{Escape}');
    expect(get(cloudConflictTarget)).not.toBeNull();

    const failure = new Error('typhon:cloudsaves.quota_exceeded: full');
    resolving.fail(failure);
    expect(await within(dialog).findByText(cloudErrorText(failure))).toBeTruthy();
    expect(dialog.isConnected).toBe(true);
  });

});
