import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { user } from '../../lib/testing/dom';

const P = vi.hoisted(() => ({ log: [] as string[] }));

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/overlay', async (o) => ({
  ...(await o<typeof import('../../lib/services/overlay')>()),
  onOverlayStatus: () => {
    P.log.push('on overlay');
    return () => P.log.push('off overlay');
  },
}));
vi.mock('../../lib/services/logsUpload', async (o) => ({
  ...(await o<typeof import('../../lib/services/logsUpload')>()),
  onLogUploadStatus: () => {
    P.log.push('on logs');
    return () => P.log.push('off logs');
  },
}));

import Settings from './Settings.svelte';

let app: HTMLDivElement;
beforeEach(() => {
  P.log = [];
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ language: 'system', uiScale: 1, overlayEnabled: true } as SettingsData);
});
afterEach(() => {
  settings.set(null);
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('the settings page', () => {
  it('listens to the logs then the overlay and stops both when it is left', async () => {
    const view = render(Settings, { target: app });
    expect(P.log).toEqual(['on logs', 'on overlay']);
    view.unmount();
    await tick();
    expect(P.log.slice(2).sort()).toEqual(['off logs', 'off overlay']);
  });

  it('closes the legal dialog by Escape and returns focus to the link', async () => {
    render(Settings, { target: app });
    const link = screen.getByRole('button', { name: msg('settings.generalPrivacyPolicyLinkLabel') });
    link.focus();
    await user().keyboard('{Enter}');
    await screen.findByRole('dialog', { name: msg('settings.generalPrivacyPolicyLinkLabel') });
    await user().keyboard('{Escape}');
    await tick();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(link);
  });
});
