import { screen, within } from '@testing-library/svelte';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { selfUpdateOutcome, selfUpdateStatus } from '../stores/selfupdate';
import { renderDom } from '../testing/dom';
import UpdateOverlay from './UpdateOverlay.svelte';
import overlaySource from './UpdateOverlay.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// jsdom has no Web Animations; the card's fades finish at once.
beforeAll(() => {
  const finished = () => ({
    cancel: () => {},
    set onfinish(done: () => void) {
      done();
    },
  });
  Object.defineProperty(Element.prototype, 'animate', { value: finished, configurable: true, writable: true });
});

afterAll(() => {
  Reflect.deleteProperty(Element.prototype, 'animate');
});

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateOutcome.set(null);
});

const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;

describe('UpdateOverlay while an update is applied', () => {
  it('says which version is installed and that the launcher restarts, in a modal dialog', () => {
    selfUpdateStatus.set(APPLYING);
    renderDom(UpdateOverlay);
    const dialog = screen.getByRole('dialog', { name: msg('ui.installingUpdateTitle') });
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(within(dialog).getByText(msg('ui.installingVersion', { version: '1.5.0' }))).toBeTruthy();
    expect(within(dialog).getByText(msg('ui.launcherWillRestartShort'))).toBeTruthy();
  });

  it('covers the whole window above the title bar, so nothing under it takes a press', () => {
    const rule = /\n {2}\.overlay \{([^}]*)\}/.exec(overlaySource)?.[1] ?? '';
    expect(rule).toMatch(/position: fixed/);
    expect(rule).toMatch(/inset: 0/);
    expect(rule).toMatch(/z-index: var\(--z-update\)/);
    expect(rule).not.toMatch(/pointer-events: none/);
  });

  it('shows nothing of it before or after applying', () => {
    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' });
    renderDom(UpdateOverlay);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByText(msg('ui.installingVersion', { version: '1.5.0' }))).toBeNull();
  });
});

describe('UpdateOverlay after a restart', () => {
  it('says the version the launcher was updated to, and the card can be hidden', async () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    const { user } = renderDom(UpdateOverlay);
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();

    await user.click(screen.getByRole('button', { name: msg('ui.hide') }));

    expect(screen.queryByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeNull();
  });

  it('says a failed install on a danger card', () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: false, error: 'boom', finishedAt: '' });
    renderDom(UpdateOverlay);
    const title = screen.getByText(msg('ui.updateFailedVersion', { version: '1.5.0' }));
    expect(title.closest('.card')?.classList.contains('card-danger')).toBe(true);
  });
});
