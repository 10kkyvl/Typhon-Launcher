import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import type { LogUploadStatus } from '../../lib/services/logsUpload';
import Settings from './Settings.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

const listeners: ((status: LogUploadStatus) => void)[] = [];
vi.mock('../../lib/services/logsUpload', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/logsUpload')>()),
  onLogUploadStatus: (handler: (status: LogUploadStatus) => void) => {
    listeners.push(handler);
    return () => listeners.splice(listeners.indexOf(handler), 1);
  },
}));

let app: HTMLDivElement;

beforeEach(() => {
  // jsdom has no ResizeObserver; the settings tabs measure their indicator with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  app = document.createElement('div');
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

function emit(status: LogUploadStatus) {
  flushSync(() => listeners.forEach((handler) => handler(status)));
}

describe('Settings: sending the logs', () => {
  it('shows the share sent, and never a percent without a total', () => {
    render(Settings, { target: app, props: { tab: 'about' } });

    emit({ state: 'sending', sentBytes: 5, totalBytes: 0 });
    expect(screen.getByText(msg('settings.aboutLogsSendingEllipsis'))).toBeTruthy();
    expect(app.textContent).not.toContain('NaN');

    emit({ state: 'sending', sentBytes: 42, totalBytes: 100 });
    expect(screen.getByText(msg('settings.aboutLogsSendingProgress', { percent: 42 }))).toBeTruthy();

    emit({ state: 'sending', sentBytes: 130, totalBytes: 100 });
    expect(screen.getByText(msg('settings.aboutLogsSendingProgress', { percent: 100 }))).toBeTruthy();
  });
});
