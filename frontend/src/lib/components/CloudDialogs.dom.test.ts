import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { cloudResolveConflict, type CloudConflict } from '../services/cloudsaves';
import { cloudConflictTarget, cloudStatus } from '../stores/cloudsaves';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import CloudConsentModal from './CloudConsentModal.svelte';
import CloudDisableModal from './CloudDisableModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
}));

const conflict = { localUpdatedAt: '2026-11-01T10:00:00Z', remoteUpdatedAt: '2026-11-02T10:00:00Z', localSize: 10, remoteSize: 20, remoteDevice: 'Deck' } as CloudConflict;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
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
  vi.mocked(cloudResolveConflict).mockReset();
  document.body.replaceChildren();
});

function showConflict(withDetails = true) {
  cloudStatus.set({
    enabled: true,
    signedIn: true,
    usedBytes: 0,
    quotaBytes: 0,
    maxBytes: 0,
    error: '',
    games: withDetails ? [{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }] : [],
  });
  cloudConflictTarget.set({ gameId: 'g1', title: 'Hades' });
  render(CloudConflictModal, { target: app });
  return screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
}

describe('CloudConflictModal', () => {
  it('states what each choice loses in a warning notice read with the dialog', () => {
    const dialog = showConflict();
    const mine = within(dialog).getByText(msg('cloudsaves.conflictKeepMineWarning'));
    const notice = mine.closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(within(notice).getByText(msg('cloudsaves.conflictTakeRemoteWarning'))).toBeTruthy();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('says it is loading the copies with a labelled spinner', () => {
    const dialog = showConflict(false);
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));
  });

  it('keeps the focus on the pressed choice, busy, and says a failure in a danger notice', async () => {
    const resolving = deferred<void>();
    vi.mocked(cloudResolveConflict).mockReturnValueOnce(resolving.promise);
    const dialog = showConflict();
    const keys = user();
    const mine = within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') });
    const remote = within(dialog).getByRole('button', { name: msg('cloudsaves.conflictTakeRemote') });
    mine.focus();

    await keys.keyboard('{Enter}');

    expect(mine.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(mine);
    expect(remote).toHaveProperty('disabled', true);

    const failure = new Error('typhon:cloudsaves.quota_exceeded: full');
    resolving.reject(failure);
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(cloudErrorText(failure));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('CloudConsentModal', () => {
  it('opens the privacy policy from a text link in its sentence', async () => {
    const onpolicy = vi.fn();
    render(CloudConsentModal, { target: app, props: { open: true, onenable: vi.fn(), onpolicy } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    const link = within(dialog).getByRole('button', { name: msg('cloudsaves.consentPolicyLink') });
    expect(link.classList.contains('text-link')).toBe(true);
    link.focus();

    await keys.keyboard('{Enter}');

    expect(onpolicy).toHaveBeenCalledTimes(1);
  });

  it('keeps the focus on "enable", busy, while it turns cloud saves on', async () => {
    const { rerender } = render(CloudConsentModal, { target: app, props: { open: true, onenable: vi.fn(), onpolicy: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    const enable = within(dialog).getByRole('button', { name: msg('cloudsaves.consentEnable') });
    enable.focus();

    await rerender({ busy: true });

    expect(enable.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(enable);
  });
});

describe('CloudDisableModal', () => {
  it('keeps the focus on the pressed choice, busy, while cloud saves turn off', async () => {
    let busy = false;
    const props = { open: true, busy, ondelete: vi.fn(), onkeep: vi.fn(() => (busy = true)) };
    const { rerender } = render(CloudDisableModal, { target: app, props });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    const keep = within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') });
    keep.focus();

    await keys.keyboard('{Enter}');
    await rerender({ busy });

    expect(keep.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(keep);
    expect(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDeleting') })).toHaveProperty('disabled', true);
  });
});
