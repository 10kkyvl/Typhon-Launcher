import { screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());

import { msg } from '../../lib/i18n';
import { dismissToast, queuedToasts, toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import Showcase from './Showcase.svelte';
import { RETRY_MS } from './ShowcaseNotices.svelte';

const ANSWER = { timeout: RETRY_MS * 4 };

const frame = async () => {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  await tick();
};

afterEach(() => {
  for (const item of [...get(toasts), ...queuedToasts()]) dismissToast(item.id);
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('the component showcase', () => {
  it('renders every section with no error and no Svelte warning', async () => {
    const warn = vi.spyOn(console, 'warn');
    const error = vi.spyOn(console, 'error');

    renderDom(Showcase);
    await frame();
    await frame();

    expect(screen.getByRole('heading', { level: 1, name: 'Component showcase' })).toBeTruthy();
    const sections = screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent);
    expect(sections).toEqual(['Actions', 'Fields', 'Status', 'Notices', 'Layers', 'Games']);
    expect(warn).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
  });

  it('lets the page make the notice retry fail, keeping focus on Retry, then succeed, moving focus to what replaced it', async () => {
    const { user } = renderDom(Showcase);
    const notice = () => screen.queryByRole('alert');
    const retry = screen.getByRole('button', { name: 'Retry' });

    retry.focus();
    await user.keyboard('{Enter}');
    expect(retry.getAttribute('aria-busy')).toBe('true');
    await waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false), ANSWER);

    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(notice()?.textContent).toContain('The list could not be loaded');
    expect(screen.getByText('Attempts: 1')).toBeTruthy();

    await user.click(screen.getByRole('radio', { name: 'Succeeds' }));
    retry.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(retry.isConnected).toBe(false), ANSWER);

    expect(notice()).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Fail the load again' }));
    expect(screen.getByText('Attempts: 2')).toBeTruthy();
  });

  it('raises a toast of each tone, and one with an action', async () => {
    const { user } = renderDom(Showcase);
    const layers = screen.getByRole('region', { name: 'Layers' });

    for (const kind of ['info', 'success', 'warning', 'danger']) {
      await user.click(within(layers).getByRole('button', { name: kind }));
    }
    await user.click(within(layers).getByRole('button', { name: 'With an action' }));

    const raised = [...get(toasts), ...queuedToasts()];
    expect(raised.map((item) => [item.kind, item.message]).sort()).toEqual(
      [
        ['danger', 'A danger toast'],
        ['info', 'A info toast'],
        ['info', 'A toast with an action'],
        ['success', 'A success toast'],
        ['warning', 'A warning toast'],
      ].sort(),
    );
    expect(raised.find((item) => item.action)?.action?.label).toBe('Undo');
  });

  it('opens a dialog and a confirm dialog from the page; a failed confirm stays open with its error', async () => {
    const { user } = renderDom(Showcase);

    const opener = screen.getByRole('button', { name: 'Open a dialog' });
    await user.click(opener);
    expect(screen.getByRole('dialog', { name: 'A dialog' })).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(opener);

    await user.click(screen.getByRole('button', { name: 'Open a confirm dialog' }));
    const confirm = screen.getByRole('dialog', { name: 'Remove the game?' });
    await user.click(within(confirm).getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(within(confirm).getByRole('alert').textContent).toContain('The action failed'), ANSWER);
    expect(screen.getByRole('dialog', { name: 'Remove the game?' })).toBe(confirm);
  });

  it('shows the saves view\'s cloud copy in its states and the save conflict banner\'s words; Sync now stays focused and busy', async () => {
    const { user } = renderDom(Showcase);
    const games = screen.getByRole('region', { name: 'Games' });

    for (const text of [msg('cloudsaves.disabledHint'), msg('cloudsaves.needsAccount'), msg('cloud.no_canonical_id'), msg('cloudsaves.stateNone'), msg('cloud.network'), msg('cloudsaves.conflictTitle')]) {
      expect(within(games).getAllByText(text).length).toBeGreaterThan(0);
    }
    expect(within(games).getAllByText(/Steam Deck/)).toHaveLength(1);

    const sync = within(games).getAllByRole('button', { name: msg('cloudsaves.syncNow') })[1];
    sync.focus();
    await user.keyboard('{Enter}');

    expect(sync.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(sync);
  });
});
