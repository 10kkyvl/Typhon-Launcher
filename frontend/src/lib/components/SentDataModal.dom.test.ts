import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/telemetryLog', async (original) => (await import('../testing/settingsDialogsTesting')).telemetryLogService(await original()));
vi.mock('../../../bindings/typhon/internal/telemetrylog', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'telemetrylog'));

import { msg } from '../i18n';
import { listSentData, type SentDataEntry } from '../services/telemetryLog';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { calls, resetBindings } from '../testing/fakeBinding';
import { deferred } from '../testing/settingsDialogsTesting';
import SentDataModal from './SentDataModal.svelte';
import source from './SentDataModal.svelte?raw';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.mocked(listSentData).mockReset();
  expect(calls).toEqual([]);
  resetBindings();
});

const ENDPOINT = 'https://telemetry.typhon.test/v1/ingest/diagnostics/very/long/path/that/does/not/fit/in/one/line?batch=20261101';
const entry = (patch: Partial<SentDataEntry> = {}): SentDataEntry => ({
  kind: 'diagnostics',
  sentAt: '2026-11-01T10:00:00Z',
  endpoint: ENDPOINT,
  payload: '{\n  "event": "launch"\n}',
  formatted: true,
  ...patch,
});

function show() {
  const rendered = render(SentDataModal, { target: app, props: { open: true } });
  return { ...rendered, dialog: screen.getByRole('dialog', { name: msg('modals.sentDataTitle') }) };
}

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

describe('SentDataModal', () => {
  it('says it is loading with a labelled spinner, a failed load in a danger notice, and a cut entry in a warning notice', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const loaded = deferred<SentDataEntry[]>();
    vi.mocked(listSentData).mockReturnValueOnce(loaded.promise);
    const { unmount, dialog } = show();
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.sentDataLoading'));

    loaded.reject(new Error('disk'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.sentDataLoadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    unmount();

    vi.mocked(listSentData).mockResolvedValueOnce([entry({ payload: '{', formatted: false })]);
    show();
    const text = await screen.findByText(msg('modals.sentDataTruncatedWarning'));
    const notice = text.closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('says nothing was sent yet with its own words', async () => {
    vi.mocked(listSentData).mockResolvedValueOnce([]);
    const { dialog } = show();
    expect(await within(dialog).findByText(msg('modals.sentDataEmptyTitle'))).toBeTruthy();
    expect(within(dialog).getByText(msg('modals.sentDataEmptyDescription'))).toBeTruthy();
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('shows each entry with its kind, time, address and payload, and copies the payload', async () => {
    vi.mocked(listSentData).mockResolvedValueOnce([entry()]);
    const { dialog } = show();
    // user-event lays its own clipboard over the navigator's: the spy goes on that one.
    const keys = user();
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    expect(await within(dialog).findByText(ENDPOINT)).toBeTruthy();
    expect(within(dialog).getByText(msg('modals.sentDataKindDiagnostics'))).toBeTruthy();

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.sentDataCopyLabel') }));

    expect(writeText).toHaveBeenCalledExactlyOnceWith(entry().payload);
    expect(get(toasts).map((item) => item.message)).toEqual([msg('modals.sentDataCopied')]);
  });

  it('shows a long address whole, with no native tooltip, and keeps the payload as selectable text', async () => {
    vi.mocked(listSentData).mockResolvedValueOnce([entry()]);
    const { dialog } = show();
    const address = await within(dialog).findByText(ENDPOINT);

    expect(dialog.querySelectorAll('[title]')).toHaveLength(0);
    expect(address.textContent).toBe(ENDPOINT);
    const payload = within(dialog).getByText((_, el) => el?.tagName === 'PRE' && el.textContent === entry().payload);
    expect(payload.closest('button, label, [role="button"]')).toBeNull();
    // The address wraps instead of being cut: no rule of the dialog keeps it on one line or hides it.
    const endpointRule = /\.endpoint\s*\{([^}]*)\}/.exec(source)?.[1] ?? '';
    expect(endpointRule).not.toMatch(/nowrap|ellipsis|overflow:\s*hidden/);
    expect(source).not.toMatch(/user-select:\s*none/);
  });

  it('loads again from the failure notice’s Retry, and gives the focus to the first entry', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(listSentData).mockRejectedValueOnce(new Error('disk')).mockResolvedValueOnce([entry()]);
    const { dialog } = show();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(await within(dialog).findByText(ENDPOINT)).toBeTruthy();
    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(listSentData).toHaveBeenCalledTimes(2);
    await vi.waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: msg('modals.sentDataCopyLabel') })));
  });

  it('keeps the notice and the focus on Retry when it fails again, and says it again', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(listSentData).mockRejectedValue(new Error('disk'));
    const { dialog } = show();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('modals.sentDataLoadFailed')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(listSentData).toHaveBeenCalledTimes(2);
  });

  it('shows the list of its latest opening when the answer of an earlier one lands last', async () => {
    const first = deferred<SentDataEntry[]>();
    const second = deferred<SentDataEntry[]>();
    vi.mocked(listSentData).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { rerender } = show();
    await rerender({ open: false });
    await rerender({ open: true });
    const dialog = screen.getByRole('dialog', { name: msg('modals.sentDataTitle') });

    second.resolve([entry({ endpoint: 'https://new.test' })]);
    expect(await within(dialog).findByText('https://new.test')).toBeTruthy();
    first.resolve([entry({ endpoint: 'https://old.test' })]);
    await first.promise;
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByText('https://old.test')).toBeNull();
    expect(within(dialog).getByText('https://new.test')).toBeTruthy();
  });

  it('keeps the list of its latest opening when the failure of an earlier one lands last', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const first = deferred<SentDataEntry[]>();
    const second = deferred<SentDataEntry[]>();
    vi.mocked(listSentData).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { rerender } = show();
    await rerender({ open: false });
    await rerender({ open: true });
    const dialog = screen.getByRole('dialog', { name: msg('modals.sentDataTitle') });

    second.resolve([entry({ endpoint: 'https://new.test' })]);
    expect(await within(dialog).findByText('https://new.test')).toBeTruthy();
    first.reject(new Error('late'));
    await expect(first.promise).rejects.toThrow('late');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(within(dialog).getByText('https://new.test')).toBeTruthy();
  });
});
