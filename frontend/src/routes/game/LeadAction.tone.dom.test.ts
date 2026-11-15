import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PrimaryKind } from '../../lib/game/view';
import { locale, msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import LeadAction from './LeadAction.svelte';

type Game = Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;
const NO_PROFILES: Game = { profiles: [], lastProfile: '', emulation: null };
const TWO_PROFILES: Game = { profiles: [{ id: 'p1', name: 'DirectX 11', executable: 'game-dx11.exe' }], lastProfile: '', emulation: null };

beforeEach(() => locale.set('en'));

afterEach(() => {
  document.body.replaceChildren();
  locale.set('ru');
});

function mount(kind: PrimaryKind, extra: { progress?: { value: number }; game?: Game } = {}) {
  const children = createRawSnippet(() => ({ render: () => '<span>Action</span>' }));
  render(LeadAction, { props: { kind, game: extra.game ?? NO_PROFILES, progress: extra.progress, onclick: vi.fn(), onpick: vi.fn(), children } });
  return screen.getByRole('button', { name: 'Action' });
}

const look = (button: HTMLElement) => ['play', 'primary', 'secondary'].find((variant) => button.classList.contains(variant));

// Only the press that launches or stops the game is painted in the game's colour (inside the page's scope);
// every other action keeps the theme's accent, and progress inside the button stays secondary.
describe("the game page's main action and the game's colour", () => {
  it('Play and Stop take the launching look', () => {
    expect(look(mount('play'))).toBe('play');
    document.body.replaceChildren();
    expect(look(mount('stop'))).toBe('play');
  });

  for (const kind of ['download', 'update', 'install-download', 'retry-download', 'other-versions', 'choose-executable'] as PrimaryKind[]) {
    it(`${kind} keeps the primary look`, () => {
      expect(look(mount(kind))).toBe('primary');
    });
  }

  it('progress inside the button stays secondary', () => {
    expect(look(mount('download', { progress: { value: 40 } }))).toBe('secondary');
  });

  it("Play's profile chevron is the other half of Play: the same look", () => {
    mount('play', { game: TWO_PROFILES });
    const chevron = screen.getByRole('button', { name: msg('games.playProfileChoose') });
    expect(look(chevron)).toBe('play');
  });
});
