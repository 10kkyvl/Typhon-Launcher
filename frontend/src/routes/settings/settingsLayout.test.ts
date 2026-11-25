import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

/**
 * settings-layout.css is global and keyed on `.settings-page`, the root of the desktop settings page and
 * of Big Picture's settings page alike. Every class it styles has a user that sits under that root; a
 * class whose last user moved to SettingsRow goes with it, and a class Big Picture's page wears stays.
 * The stylesheet's text is read from disk: the test run empties CSS imports.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const sheet = fs.readFileSync(new URL('./settings-layout.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

// Each class the sheet styles, with the files whose markup wears it under `.settings-page`.
const USERS: Record<string, string[]> = {
  // The container both settings pages measure.
  'settings-page': ['routes/settings/Settings.svelte', 'lib/bigpicture/components/SettingsPage.svelte'],
  // One column, two on a wide page: the four tabs and Big Picture's grid of cards.
  'settings-grid': [
    'routes/settings/GeneralTab.svelte',
    'routes/settings/DownloadsTab.svelte',
    'routes/settings/AppearanceTab.svelte',
    'routes/settings/AboutTab.svelte',
    'lib/bigpicture/components/SettingsPage.svelte',
  ],
  'settings-column': [
    'routes/settings/GeneralTab.svelte',
    'routes/settings/DownloadsTab.svelte',
    'routes/settings/AppearanceTab.svelte',
    'routes/settings/AboutTab.svelte',
  ],
  // A card may shrink inside its column.
  card: ['lib/components/Card.svelte'],
  // The rows not yet on SettingsRow wrap their control under the words: the appearance tab's own rows
  // and the Discord card's unlinked intro.
  row: ['routes/settings/AppearanceTab.svelte', 'routes/settings/DiscordCard.svelte'],
  'row-text': ['routes/settings/AppearanceTab.svelte'],
  'row-sub': ['routes/settings/AppearanceTab.svelte'],
};

const styled = [...new Set([...sheet.matchAll(/\.([a-zA-Z][\w-]*)/g)].map((m) => m[1]))].sort();

function wears(text: string, name: string): boolean {
  const markup = text.replace(/<script[^>]*>[\s\S]*?<\/script>|<style[^>]*>[\s\S]*?<\/style>/g, '');
  const word = String.raw`(?<![\w-])${name}(?![\w-])`;
  return new RegExp(String.raw`\bclass="[^"]*${word}`).test(markup) || new RegExp(String.raw`\bclass:${name}\b`).test(markup);
}

describe('settings-layout.css', () => {
  it('styles only classes that something under the settings page still wears', () => {
    expect(styled).toEqual(Object.keys(USERS).sort());
  });

  it('is worn by each user named for it, Big Picture included', () => {
    const missing = Object.entries(USERS).flatMap(([name, paths]) =>
      paths.filter((path) => !wears(frontendSources[path] ?? '', name)).map((path) => `${path}: .${name}`),
    );
    expect(missing).toEqual([]);
  });

  it("keeps Big Picture's page and grid on the shared container", () => {
    const page = frontendSources['lib/bigpicture/components/SettingsPage.svelte'];
    expect(page).toMatch(/class="bp-page settings-page"/);
    expect(page).toMatch(/class="bp-grid settings-grid"/);
    expect(sheet).toMatch(/\.settings-page\s*\{[^}]*container:\s*settings-page \/ inline-size/);
    expect(sheet).toMatch(/@container settings-page \(min-width: 110rem\)\s*\{\s*\.settings-page \.settings-grid\s*\{\s*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\);/);
  });
});
