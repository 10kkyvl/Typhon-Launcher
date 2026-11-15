import { derived, readable, writable, type Readable } from 'svelte/store';
import type { Attachment } from 'svelte/attachments';
import { dominant, gameToneOf, toHex, toneVars, type GameTone, type RGB, type ToneTheme } from '../art/tone';
import { readPixels } from '../backdrop/pixels';
import { toneSource } from '../backdrop/source';
import { themeApplied } from '../theme/apply';
import { resolveColors } from '../theme/resolveColors';
import { libraryGames } from './library';
import { artAnswered, gameArt } from './metadata';

/**
 * A game's own colour, for the controls that launch it (Play and its Stop, the round Play of a card), the
 * game page's tab marker, and the marks drawn for it (a progress fill, a speed graph's line). Read once per game and picture from the art (lib/art/tone.ts), kept for the
 * session as three numbers per game, and clamped into the theme on every theme change without reading a
 * picture again. Until it is known, and when it cannot be known, a subscriber gets null and the controls
 * keep the theme's accent: no stand-in hue first.
 */

/** The long edge a picture is read at: enough pixels for the vote of hues, no more. */
const SAMPLE_SIDE = 64;

// Per key: the colour read (null: none to take); `asked` holds the pictures it is read from.
const sampled = writable<Readonly<Record<string, RGB | null>>>({});
const asked = new Map<string, string>();

/**
 * The first picture that gives a colour wins. The colour comes only from a picture that was read: a game
 * with none, or whose pictures do not load, are tainted or grey, keeps the theme's accent, since whatever
 * a screen draws in their place is not something this store can see.
 */
async function sample(pictures: readonly string[]): Promise<RGB | null> {
  for (const url of pictures) {
    const read = await readPixels(url, SAMPLE_SIDE);
    const rgb = read.ok ? dominant(read.data) : null;
    if (rgb) return rgb;
  }
  return null;
}

// One read per game and set of pictures, however many controls ask; a new picture (or a failed one the
// art store replaced) is read again. An answer for pictures the game no longer has is dropped.
function ensure(key: string, pictures: readonly string[]) {
  const from = pictures.join('\n');
  if (asked.get(key) === from) return;
  asked.set(key, from);
  void sample(pictures).then((rgb) => {
    if (asked.get(key) !== from) return;
    sampled.update((all) => ({ ...all, [key]: rgb }));
  });
}

/**
 * The page's --bg and --text, and the grounds a mark is drawn on (the panel --surface-2 and the progress
 * track --hover-strong), as the window paints them: read in one pass each time the window takes a new look,
 * never per progress tick. A ground that does not resolve is left out and the mark is held on the rest.
 */
const theme: Readable<ToneTheme | null> = derived(themeApplied, () => {
  const colors = resolveColors({}, ['--bg', '--text', '--surface-2', '--hover-strong']);
  const bg = colors['--bg'];
  const text = colors['--text'];
  if (!bg || !text) return null;
  const tone: ToneTheme = { bg: toHex([bg[0], bg[1], bg[2]]), text: toHex([text[0], text[1], text[2]]) };
  const panel = colors['--surface-2'];
  if (panel) tone.panel = toHex([0, 1, 2].map((i) => panel[i] * panel[3] + bg[i] * (1 - panel[3])) as RGB);
  const track = colors['--hover-strong'];
  if (track) tone.track = track;
  return tone;
});

/** The game's colour clamped into the current theme; null for no game, while unknown, and when there is none. */
export function gameTone(id: string | null | undefined): Readable<GameTone | null> {
  if (!id) return readable(null);
  return derived([libraryGames, gameArt, artAnswered, sampled, theme], ([games, art, answered, colours, current]) => {
    const source = toneSource(id, games, art, answered);
    if (!source) return null;
    ensure(source.key, source.pictures);
    const rgb = colours[source.key];
    return rgb && current ? gameToneOf(rgb, current) : null;
  });
}

/**
 * Paints a subtree in the game's colour: the `--game-*` properties inline and the `game-tone` class, which
 * shell.css turns into the colours Play reads. Nothing is set until the colour is known, and everything
 * goes on a change of game and when the node is destroyed.
 *
 *   <div {@attach gameScope(game.id)}>
 */
export function gameScope(id: string | null | undefined): Attachment<HTMLElement> {
  return (node) => {
    let applied: string[] = [];
    let shown = '';
    const clear = () => {
      for (const name of applied) node.style.removeProperty(name);
      applied = [];
      shown = '';
      node.classList.remove('game-tone');
    };
    const unsubscribe = gameTone(id).subscribe((tone) => {
      const vars = toneVars(tone);
      const next = JSON.stringify(vars);
      if (next === shown) return;
      clear();
      for (const [name, value] of Object.entries(vars)) node.style.setProperty(name, value);
      applied = Object.keys(vars);
      shown = next;
      if (tone) node.classList.add('game-tone');
    });
    return () => {
      unsubscribe();
      clear();
    };
  };
}
