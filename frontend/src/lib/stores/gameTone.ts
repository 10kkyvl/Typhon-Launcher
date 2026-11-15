import { derived, readable, writable, type Readable } from 'svelte/store';
import type { Attachment } from 'svelte/attachments';
import { artHorizon } from '../art/scene';
import { dominant, gameToneOf, hslToRgb, toHex, toneVars, type GameTone, type RGB, type ToneTheme } from '../art/tone';
import { readPixels } from '../backdrop/pixels';
import { toneSource } from '../backdrop/source';
import { themeApplied } from '../theme/apply';
import { resolveColors } from '../theme/resolveColors';
import { libraryGames } from './library';
import { artAnswered, gameArt } from './metadata';

/**
 * A game's own colour, for the controls that launch it (Play and its Stop, the round Play of a card) and
 * the game page's tab marker. Read once per game and picture from the art (lib/art/tone.ts), kept for the
 * session as three numbers per game, and clamped into the theme on every theme change without reading a
 * picture again. Until it is known, and when it cannot be known, a subscriber gets null and the controls
 * keep the theme's accent: no stand-in hue first.
 */

/** The long edge a picture is read at: enough pixels for the vote of hues, no more. */
const SAMPLE_SIDE = 64;

// Per key: the colour read (null: none to take); `asked` holds the pictures it is read from.
const sampled = writable<Readonly<Record<string, RGB | null>>>({});
const asked = new Map<string, string>();

function horizon(seed: string): RGB {
  const [h, s, l] = artHorizon(seed);
  return hslToRgb(h, s / 100, l / 100);
}

/**
 * The first picture that gives a colour wins. A game none of whose pictures loads (or that has none) is
 * drawn by its generated art on the card, so it takes that art's colour; a picture that shows but cannot
 * be read (tainted, grey, no canvas) leaves the theme's accent, never a hue the player sees nowhere.
 */
async function sample(pictures: readonly string[], seed: string): Promise<RGB | null> {
  let nothingShown = true;
  for (const url of pictures) {
    const read = await readPixels(url, SAMPLE_SIDE);
    if (read.ok) {
      const rgb = dominant(read.data);
      if (rgb) return rgb;
      nothingShown = false;
    } else if (read.reason !== 'load') {
      nothingShown = false;
    }
  }
  return nothingShown ? horizon(seed) : null;
}

// One read per game and set of pictures, however many controls ask; a new picture (or a failed one the
// art store replaced) is read again. An answer for pictures the game no longer has is dropped.
function ensure(key: string, pictures: readonly string[]) {
  const from = pictures.join('\n');
  if (asked.get(key) === from) return;
  asked.set(key, from);
  void sample(pictures, key).then((rgb) => {
    if (asked.get(key) !== from) return;
    sampled.update((all) => ({ ...all, [key]: rgb }));
  });
}

/** The page's --bg and --text as the window paints them, read again each time the window takes a new look. */
const theme: Readable<ToneTheme | null> = derived(themeApplied, () => {
  const colors = resolveColors({}, ['--bg', '--text']);
  const bg = colors['--bg'];
  const text = colors['--text'];
  if (!bg || !text) return null;
  return { bg: toHex([bg[0], bg[1], bg[2]]), text: toHex([text[0], text[1], text[2]]) };
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
