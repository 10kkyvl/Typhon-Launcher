// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { launchesInFlight, playGame, stopGame } from '../../lib/services/library';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { toast } from '../../lib/stores/toasts';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { carousel, played, recentSection, results } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => {
  const { writable: store } = await import('svelte/store');
  return {
    ...(await importOriginal<typeof import('../../lib/services/library')>()),
    getGames: vi.fn(async () => []),
    getCollections: vi.fn(async () => []),
    playGame: vi.fn(async () => {}),
    stopGame: vi.fn(async () => {}),
    // A launch another screen started: the service's own set, written by the test.
    launchesInFlight: store<ReadonlySet<string>>(new Set()),
  };
});
vi.mock('../../lib/stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(() => 0),
}));

const inFlight = launchesInFlight as unknown as ReturnType<typeof writable<ReadonlySet<string>>>;

// The carousel holds the three last played, the row the two before them, the grid all five by title.
const games = [
  played('g-alpha', 'Alpha', 9),
  played('g-bravo', 'Bravo', 8),
  played('g-charlie', 'Charlie', 7),
  played('g-delta', 'Delta', 6),
  played('g-echo', 'Echo', 5),
];

type Surface = 'carousel' | 'capsule' | 'card';

/** The Play or Stop control of the first game a surface shows, with that game's id. */
function controlOn(surface: Surface, name: string): { control: HTMLElement; id: string } {
  if (surface === 'carousel') return { control: within(carousel() as HTMLElement).getByRole('button', { name }), id: 'g-alpha' };
  if (surface === 'capsule') return { control: within(recentSection() as HTMLElement).getAllByRole('button', { name })[0], id: 'g-delta' };
  return { control: within(results() as HTMLElement).getAllByRole('button', { name })[0], id: 'g-alpha' };
}

function held() {
  let release = () => {};
  const promise = new Promise<void>((resolve) => (release = resolve));
  return { promise, release };
}

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryView.set('grid');
  libraryLoaded.set(true);
  libraryGames.set(games);
  inFlight.set(new Set());
  vi.mocked(playGame).mockReset();
  vi.mocked(stopGame).mockReset();
  vi.mocked(toast).mockClear();
});

afterEach(() => {
  libraryGames.set([]);
  runningGames.set(new Set());
  document.body.replaceChildren();
});

const surfaces: Surface[] = ['carousel', 'capsule', 'card'];

describe('Library: Play while the launch is in flight', () => {
  it.each(surfaces)('on a %s: a second press sends nothing and says nothing; the control stays busy and focused', async (surface) => {
    const launch = held();
    vi.mocked(playGame).mockReturnValueOnce(launch.promise);
    const { user } = renderDom(Library);
    const { control, id } = controlOn(surface, msg('ui.play'));
    control.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');
    await user.click(control);

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(playGame).toHaveBeenCalledWith(id);
    expect(toast).not.toHaveBeenCalled();
    expect(control.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(control);

    launch.release();
    await vi.waitFor(() => expect(control.getAttribute('aria-busy')).toBeNull());
  });

  it.each(surfaces)('on a %s: a launch another screen started shows busy, and a press sends nothing', async (surface) => {
    const { user } = renderDom(Library);
    const { control, id } = controlOn(surface, msg('ui.play'));
    inFlight.set(new Set([id]));
    await tick();

    await user.click(control);

    expect(playGame).not.toHaveBeenCalled();
    expect(control.getAttribute('aria-busy')).toBe('true');
  });
});

describe('Library: Stop', () => {
  beforeEach(() => runningGames.set(new Set(['g-alpha', 'g-delta'])));

  it.each(surfaces)('on a %s: busy while it is in flight, a second press sends nothing', async (surface) => {
    const stopping = held();
    vi.mocked(stopGame).mockReturnValueOnce(stopping.promise);
    const { user } = renderDom(Library);
    const { control, id } = controlOn(surface, msg('ui.stop'));
    control.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(stopGame).toHaveBeenCalledTimes(1);
    expect(stopGame).toHaveBeenCalledWith(id);
    expect(playGame).not.toHaveBeenCalled();
    expect(control.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(control);
    stopping.release();
  });

  it.each([
    ['an uncoded failure says it could not stop', new Error('Не удалось: сырой текст'), () => msg('games.errorStopFailed')],
    ['a coded failure says its code', new Error('typhon:library.game_not_found: сырой текст'), () => msg('library.game_not_found')],
  ])('%s, never that it could not launch', async (_name, err, expected) => {
    vi.mocked(stopGame).mockRejectedValueOnce(err);
    const { user } = renderDom(Library);

    await user.click(controlOn('carousel', msg('ui.stop')).control);

    await vi.waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(vi.mocked(toast).mock.calls[0]).toEqual([expected(), 'danger']);
  });
});
