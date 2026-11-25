import { render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// The about tab in the desktop app: the logs can be sent and the legal list is asked for.
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getAppInfo: vi.fn(async () => ({ version: '1.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' })),
  getSystemInfo: vi.fn(async () => {
    throw new Error('no system info');
  }),
  getWineStatus: vi.fn(async () => ({ required: false, installed: false, version: '' })),
}));
vi.mock('../../lib/services/legal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/legal')>()),
  listLegalDocuments: vi.fn(),
}));
vi.mock('../../lib/services/logsUpload', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/logsUpload')>()),
  onLogUploadStatus: () => () => {},
  sendLogs: vi.fn(),
}));
vi.mock('../../lib/services/overlay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/overlay')>()),
  overlayStatus: vi.fn(async () => ({ supported: false, enabled: false, hotkey: '', error: '' })),
  onOverlayStatus: () => () => {},
}));
vi.mock('../../lib/stores/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/cloudsaves')>()),
  refreshCloudStatus: vi.fn(async () => {}),
}));

import { msg } from '../../lib/i18n';
import { listLegalDocuments } from '../../lib/services/legal';
import { sendLogs } from '../../lib/services/logsUpload';
import { overlayStatus } from '../../lib/services/overlay';
import { user } from '../../lib/testing/dom';
import Settings from './Settings.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.mocked(listLegalDocuments).mockResolvedValue([]);
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  vi.mocked(sendLogs).mockReset();
  vi.mocked(listLegalDocuments).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('Settings: the overlay status in the app', () => {
  // A failed overlay is a state of the row, refreshed by events: it sits on a danger plate inside the
  // row's status region, which says a change politely and says nothing on opening the page.
  it('shows a failed overlay on a danger notice inside the status line, not as red text', async () => {
    vi.mocked(overlayStatus).mockResolvedValueOnce({ supported: true, enabled: true, hotkey: 'Alt+`', error: 'hook failed' });
    render(Settings, { target: app });
    const text = await screen.findByText(msg('settings.overlaySaveErrorPlain'));
    expect(screen.queryByText('hook failed')).toBeNull();
    expect(text.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
    const region = text.closest('[role="status"]') as HTMLElement;
    expect(region).not.toBeNull();
    expect(region.closest('[data-tone]')).toBeNull();
    expect(text.closest('[role="alert"]')).toBeNull();
  });

  it('keeps a working overlay as a plain status line', async () => {
    vi.mocked(overlayStatus).mockResolvedValueOnce({ supported: true, enabled: true, hotkey: 'Alt+`', error: '' });
    render(Settings, { target: app });
    const text = await screen.findByText(msg('settings.overlayStatusOn', { hotkey: 'Alt+`' }));
    expect(text.closest('[data-tone]')).toBeNull();
    expect(text.closest('[role="status"]')).not.toBeNull();
  });
});

describe('Settings: the about tab in the app', () => {
  it('reports a failed send of the logs on a danger notice in its row', async () => {
    vi.mocked(sendLogs).mockRejectedValueOnce(new Error('offline'));
    render(Settings, { target: app, props: { tab: 'about' } });
    await user().click(screen.getByRole('button', { name: msg('settings.aboutLogsSendButton') }));
    const dialog = await screen.findByRole('dialog');
    await user().click(within(dialog).getByRole('button', { name: msg('settings.aboutLogsSendConfirmButton') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('settings.aboutLogsSendFailedHint'));
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
    // Its row: the nearest block that holds the send button too, and not the save button above it.
    const send = screen.getByRole('button', { name: msg('settings.aboutLogsSendButton') });
    let row: HTMLElement | null = alert.parentElement;
    while (row && !row.contains(send)) row = row.parentElement;
    expect(row).not.toBeNull();
    expect(row?.contains(screen.getByRole('button', { name: msg('settings.aboutLogsDownloadButton') }))).toBe(false);
  });

  it('reports a legal list that cannot be loaded as an error that is announced', async () => {
    vi.mocked(listLegalDocuments).mockRejectedValueOnce(new Error('broken'));
    render(Settings, { target: app, props: { tab: 'about' } });
    const alert = await waitFor(() => {
      const found = screen.getAllByRole('alert').find((el) => el.textContent?.includes(msg('settings.aboutLegalLoadError')));
      expect(found).toBeDefined();
      return found as HTMLElement;
    });
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });
});
