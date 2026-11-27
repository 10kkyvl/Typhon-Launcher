import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getUpdateHistory, startUpdate, type Update, type UpdateHistory } from '../services/updates';
import type { MoveJob } from '../services/relocate';
import { moves } from '../stores/relocate';
import { toasts } from '../stores/toasts';
import { renderDom } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import UpdateCard from './UpdateCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/updates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/updates')>()),
  getUpdateHistory: vi.fn(async () => []),
  startUpdate: vi.fn(),
}));

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  moves.set([]);
  vi.mocked(getUpdateHistory).mockReset();
  vi.mocked(startUpdate).mockReset();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function move(patch: Partial<MoveJob> = {}): MoveJob {
  return { id: 'm1', scope: 'game', stage: 'copy', gameId: 'g', title: 'Game', ...patch } as MoveJob;
}

function update(patch: Partial<Update> = {}): Update {
  return {
    gameId: 'g', title: 'Game', state: 'update_available', planning: false, progress: 0, canRollback: false, checkedAt: '',
    availability: {
      available: true, kind: 'update', gameId: 'g', installedReleaseId: 'r1', targetReleaseId: 'r2', installedVersion: '1', targetVersion: '2',
      confidence: 1, strategy: 'full_release', estimatedDownloadBytes: 10, requiresFullInstall: true, patchCount: 0, targetSize: 10, reason: '',
    },
    ...patch,
  } as Update;
}

const plan = {
  strategy: 'full_release', downloadBytes: 10, reusedBytes: 0, requiredDiskBytes: 20, installedVersion: '1', targetVersion: '2',
  backupAvailable: true, rollbackAvailable: true, steps: [{ label: 'Download', bytes: 10 }],
};

describe('UpdateCard', () => {
  it('shows the plan through a Details disclosure that says what it opens', async () => {
    const { user } = renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: false });
    const details = screen.getByRole('button', { name: msg('ui.details') });
    expect(details.getAttribute('aria-expanded')).toBe('false');
    expect(details.hasAttribute('aria-pressed')).toBe(false);
    details.focus();

    await user.keyboard('{Enter}');

    expect(details.getAttribute('aria-expanded')).toBe('true');
    const panel = document.getElementById(details.getAttribute('aria-controls') ?? '');
    expect(panel?.textContent).toContain(msg('ui.spaceNeeded'));
  });

  it('shows the size being calculated as the base spinner named by that', () => {
    renderDom(UpdateCard, { update: update({ planning: true }), running: false });
    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('ui.calculatingDownloadSize'));
  });

  it('states a running game and a failed update in notices of the card', () => {
    renderDom(UpdateCard, { update: update({ error: 'disk_full' } as Partial<Update>), running: true });
    expect(screen.getByText(msg('ui.gameRunningBeforeUpdate')).closest('.notice')).toBeTruthy();
    expect(document.querySelector('.notice[data-tone="danger"]')).toBeTruthy();
    expect(document.querySelector('p.error')).toBeNull();
  });

  it('turns Update off while the game moves and says why, as the game page does', async () => {
    moves.set([move()]);
    const { user } = renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: false });

    const action = screen.getByRole('button', { name: msg('ui.updateAction') });
    expect(action.hasAttribute('disabled')).toBe(true);
    expect(screen.getByText(msg('games.mainActionMoving')).closest('.notice')).toBeTruthy();
    await user.click(action);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('keeps Update on while another game moves or the game\'s own move is over', () => {
    moves.set([move({ gameId: 'other' })]);
    renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: false });
    expect(screen.getByRole('button', { name: msg('ui.updateAction') }).hasAttribute('disabled')).toBe(false);
    expect(screen.queryByText(msg('games.mainActionMoving'))).toBeNull();
  });

  it('keeps Update on once the game\'s move has failed', () => {
    moves.set([move({ stage: 'failed' })]);
    renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: false });
    expect(screen.getByRole('button', { name: msg('ui.updateAction') }).hasAttribute('disabled')).toBe(false);
  });

  it('says the history could not be loaded when it fails, and opens no empty dialog', async () => {
    vi.mocked(getUpdateHistory).mockRejectedValue(new Error('backend down'));
    const { user } = renderDom(UpdateCard, { update: update(), running: false });

    await user.click(screen.getByRole('button', { name: msg('ui.history') }));

    await vi.waitFor(() => expect(get(toasts).map((item) => [item.message, item.kind])).toEqual([[msg('ui.updateHistoryFailed'), 'danger']]));
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});

const footerOf = (dialog: HTMLElement) => within(dialog).getAllByRole('button').filter((button) => !button.closest('[data-skip-autofocus]'));
const reasonOf = (button: HTMLElement) =>
  (button.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

describe('UpdateCard: the update question', () => {
  async function ask(props: { running?: boolean } = {}) {
    const rendered = renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: props.running ?? false });
    await rendered.user.click(screen.getByRole('button', { name: msg('ui.updateAction') }));
    return { ...rendered, dialog: screen.getByRole('dialog', { name: msg('ui.updateGameTitle') }) };
  }

  it('opens with the focus on Cancel, says the plan, and has no native title', async () => {
    const { dialog } = await ask();

    expect(footerOf(dialog).map((button) => button.textContent?.trim())).toEqual([msg('common.cancel'), msg('ui.updateAction')]);
    expect(document.activeElement).toBe(footerOf(dialog)[0]);
    expect(dialog.textContent).toContain(msg('ui.backupWillBeCreated'));
    expect(dialog.querySelectorAll('[title]').length).toBe(0);
  });

  it('starts the update once for a press and closes; Escape starts nothing', async () => {
    vi.mocked(startUpdate).mockResolvedValue(undefined);
    const first = await ask();

    await first.user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(vi.mocked(startUpdate)).not.toHaveBeenCalled();

    await first.user.click(screen.getByRole('button', { name: msg('ui.updateAction') }));
    const dialog = screen.getByRole('dialog', { name: msg('ui.updateGameTitle') });
    await first.user.click(footerOf(dialog)[1]);

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(vi.mocked(startUpdate).mock.calls).toEqual([['g']]);
  });

  it('holds Update with the reason when the game starts while the question is open, and starts nothing', async () => {
    const { dialog, user, rerender } = await ask();

    await rerender({ update: update({ plan } as Partial<Update>), running: true });

    const go = footerOf(dialog)[1];
    expect(go.hasAttribute('disabled')).toBe(false);
    expect(go.getAttribute('aria-disabled')).toBe('true');
    expect(reasonOf(go)).toContain(msg('ui.gameRunningBeforeUpdate'));
    await user.click(go);
    expect(vi.mocked(startUpdate)).not.toHaveBeenCalled();
    expect(dialog.isConnected).toBe(true);
  });

  it('holds Update with the reason when the update starts elsewhere while the question is open', async () => {
    const { dialog, user, rerender } = await ask();

    await rerender({ update: update({ plan, state: 'updating' } as Partial<Update>), running: false });

    expect(reasonOf(footerOf(dialog)[1])).toContain(msg('ui.updateInProgress'));
    await user.click(footerOf(dialog)[1]);
    expect(vi.mocked(startUpdate)).not.toHaveBeenCalled();
  });

  it('holds Update with the reason when the game starts moving while the question is open', async () => {
    const { dialog, user } = await ask();

    moves.set([move()]);
    await vi.waitFor(() => expect(footerOf(dialog)[1].getAttribute('aria-disabled')).toBe('true'));

    expect(reasonOf(footerOf(dialog)[1])).toContain(msg('games.mainActionMoving'));
    await user.click(footerOf(dialog)[1]);
    expect(vi.mocked(startUpdate)).not.toHaveBeenCalled();
  });
});

describe('UpdateCard: the history', () => {
  const entry = { id: 'h1', gameId: 'g', fromVersion: '1', toVersion: '2', strategy: 'full_release', downloadBytes: 10, startedAt: '2026-11-01T10:00:00Z', completedAt: null, status: 'completed' } as UpdateHistory;

  it('is read once for two presses, History busy and focused while it is', async () => {
    const reading = held<UpdateHistory[]>();
    vi.mocked(getUpdateHistory).mockReturnValueOnce(reading.promise);
    const { user } = renderDom(UpdateCard, { update: update(), running: false });
    const history = screen.getByRole('button', { name: msg('ui.history') });
    history.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(vi.mocked(getUpdateHistory).mock.calls).toEqual([['g']]);
    expect(history.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(history);
    reading.resolve([entry]);
    const dialog = await screen.findByRole('dialog', { name: msg('ui.updateHistoryTitle') });
    expect(within(dialog).getByText('1 → 2')).toBeTruthy();
    expect(dialog.querySelectorAll('[title]').length).toBe(0);
  });

  it('says each row’s status and method in words, never Go’s codes', async () => {
    const row = (status: string, strategy = 'full_release') => ({ ...entry, id: `${status}-${strategy}`, status, strategy });
    vi.mocked(getUpdateHistory).mockResolvedValueOnce([
      row('completed'),
      row('failed', 'patch_chain'),
      row('running', 'torrent_reuse'),
      row('rolled_back'),
      row('someday_new', 'someday_strategy'),
    ]);
    const { user } = renderDom(UpdateCard, { update: update(), running: false });

    await user.click(screen.getByRole('button', { name: msg('ui.history') }));

    const dialog = await screen.findByRole('dialog', { name: msg('ui.updateHistoryTitle') });
    const rows = [...dialog.querySelectorAll('li')].map((item) => [...item.querySelectorAll('.muted')].map((part) => part.textContent?.trim()));
    expect(rows.map(([strategy, , status]) => [strategy, status])).toEqual([
      [msg('state.updatesStrategyFullRelease'), msg('ui.updateHistoryStatusCompleted')],
      [msg('state.updatesStrategyPatchChain'), msg('ui.updateHistoryStatusFailed')],
      [msg('state.updatesStrategyTorrentReuse'), msg('ui.updateHistoryStatusRunning')],
      [msg('state.updatesStrategyFullRelease'), msg('ui.updateHistoryStatusRolledBack')],
      [msg('state.updatesStrategyUnknown'), msg('ui.updateHistoryStatusUnknown')],
    ]);
    for (const code of ['completed', 'failed', 'running', 'rolled_back', 'someday_new', 'someday_strategy']) expect(dialog.textContent).not.toContain(code);
  });

  it('says there is none with an empty state, and Close gives the focus back to History', async () => {
    const { user } = renderDom(UpdateCard, { update: update(), running: false });
    const history = screen.getByRole('button', { name: msg('ui.history') });

    await user.click(history);

    const dialog = await screen.findByRole('dialog', { name: msg('ui.updateHistoryTitle') });
    expect(within(dialog).getByText(msg('ui.noUpdatesYet'))).toBeTruthy();
    await user.click(footerOf(dialog)[0]);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(history);
  });

  it('opens nothing when the history answers after the card turned to another game', async () => {
    const reading = held<UpdateHistory[]>();
    vi.mocked(getUpdateHistory).mockReturnValueOnce(reading.promise);
    const { user, rerender } = renderDom(UpdateCard, { update: update(), running: false });
    await user.click(screen.getByRole('button', { name: msg('ui.history') }));

    const other = update({ gameId: 'g2', title: 'Other' });
    await rerender({ update: { ...other, availability: { ...other.availability, gameId: 'g2' } }, running: false });
    reading.resolve([entry]);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getByRole('button', { name: msg('ui.history') }).getAttribute('aria-busy')).toBeNull();
  });

  it('says nothing when the history fails after the card turned to another game', async () => {
    const reading = held<UpdateHistory[]>();
    vi.mocked(getUpdateHistory).mockReturnValueOnce(reading.promise);
    const { user, rerender } = renderDom(UpdateCard, { update: update(), running: false });
    await user.click(screen.getByRole('button', { name: msg('ui.history') }));

    const other = update({ gameId: 'g2', title: 'Other' });
    await rerender({ update: { ...other, availability: { ...other.availability, gameId: 'g2' } }, running: false });
    reading.reject(new Error('backend down'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts)).toEqual([]);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
