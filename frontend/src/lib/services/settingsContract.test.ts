import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));

import { getSettings } from './settings';
import { DEFAULT_OVERLAY_HOTKEY, OVERLAY_HOTKEYS } from './overlay';
import { DEFAULT_PRESENCE, PRESENCE_STATUSES } from './online';
import { bindingSources, frontendSources, goSources } from '../testing/sources';

const GO = 'internal/settings/settings.go';

function goSource(): string {
  const source = goSources[GO];
  expect(source, GO).toBeTypeOf('string');
  return source;
}

function goConstants(): Map<string, string | number> {
  const constants = new Map<string, string | number>();
  for (const match of goSource().matchAll(/^\t([A-Za-z]\w*)\s*=\s*(?:"([^"]*)"|(-?\d+))\s*(?:\/\/.*)?$/gm)) {
    constants.set(match[1], match[2] !== undefined ? match[2] : Number(match[3]));
  }
  return constants;
}

function constantsWithPrefix(prefix: string): Array<string | number> {
  return [...goConstants()]
    .filter(([name]) => name.startsWith(prefix))
    .map(([, value]) => value);
}

interface GoField {
  name: string;
  type: string;
  json: string;
}

function goFields(): GoField[] {
  const body = /type Settings struct \{([\s\S]*?)\n\}/.exec(goSource())?.[1] ?? '';
  return [...body.matchAll(/^\t(\w+)\s+(\w+)\s+`json:"(\w+)"`/gm)].map((match) => ({
    name: match[1],
    type: match[2],
    json: match[3],
  }));
}

function goDefaults(): Map<string, string | number | boolean> {
  const body = /func Defaults\(\) Settings \{\s*return Settings\{([\s\S]*?)\n\t\}\n\}/.exec(goSource())?.[1] ?? '';
  const constants = goConstants();
  const byName = new Map<string, string | number | boolean>();
  for (const line of body.split('\n')) {
    const match = /^\t\t(\w+):\s*(.+?),\s*(?:\/\/.*)?$/.exec(line);
    if (!match) continue;
    const raw = match[2].trim();
    if (raw === 'true' || raw === 'false') byName.set(match[1], raw === 'true');
    else if (/^-?\d+(\.\d+)?$/.test(raw)) byName.set(match[1], Number(raw));
    else if (/^"[^"]*"$/.test(raw)) byName.set(match[1], raw.slice(1, -1));
    else if (constants.has(raw)) byName.set(match[1], constants.get(raw)!);
    else throw new Error(`Defaults(): cannot read ${match[1]}: ${raw}`);
  }
  const defaults = new Map<string, string | number | boolean>();
  for (const field of goFields()) {
    if (byName.has(field.name)) {
      defaults.set(field.json, byName.get(field.name)!);
    } else if (field.type === 'bool') {
      defaults.set(field.json, false);
    } else if (field.type === 'string') {
      defaults.set(field.json, '');
    } else {
      defaults.set(field.json, 0);
    }
  }
  return defaults;
}

function optionIds(source: string, from: string, to: string): string[] {
  const start = source.indexOf(from);
  expect(start, `marker ${from}`).toBeGreaterThan(-1);
  const end = source.indexOf(to, start);
  expect(end, `marker ${to}`).toBeGreaterThan(start);
  return [...source.slice(start, end).matchAll(/\bid:\s*'([^']*)'/g)].map((match) => match[1]);
}

const settingsPage = frontendSources['routes/settings/Settings.svelte'];
const networkCard = frontendSources['routes/settings/NetworkSettingsCard.svelte'];

describe('settings across the bridge', () => {
  it('reads the Go struct it is guarding', () => {
    expect(goFields().length).toBeGreaterThan(40);
  });

  it('keeps the generated model on the Go json names', () => {
    const generated = bindingSources['typhon/internal/settings/models.ts'];
    const names = [...generated.matchAll(/^\s*"(\w+)"\??:/gm)].map((match) => match[1]).sort();
    expect(names, 'regenerate with: wails3 task common:generate:bindings').toEqual(
      goFields().map((field) => field.json).sort(),
    );
  });

  it('gives the browser fallback the same defaults as Go', async () => {
    const mine = (await getSettings()) as unknown as Record<string, unknown>;
    const theirs = goDefaults();
    const different = [...theirs]
      .filter(([name, value]) => mine[name] !== value)
      .map(([name, value]) => `${name}: Go ${JSON.stringify(value)}, fallback ${JSON.stringify(mine[name])}`);
    expect(different).toEqual([]);
  });
});

describe('settings choices', () => {
  it('offers every download cleanup policy Go accepts, and no other', () => {
    const ids = optionIds(settingsPage, 'function cleanupPolicyOptionsList', 'const cleanupPolicyOptions');
    expect(ids.sort()).toEqual(constantsWithPrefix('Cleanup').map(String).sort());
  });

  it('offers every source refresh interval Go accepts, and no other', () => {
    const ids = optionIds(settingsPage, 'function sourceRefreshOptionsList', 'const sourceRefreshOptions');
    expect(ids.sort()).toEqual(constantsWithPrefix('Refresh').map(String).sort());
  });

  it('offers every keep-previous-version choice Go accepts, and no other', () => {
    const ids = optionIds(settingsPage, 'function keepPreviousOptionsList', 'const keepPreviousOptions');
    expect(ids.sort()).toEqual(constantsWithPrefix('KeepPrevious').map(String).sort());
  });

  it('offers every interface language Go accepts, and no other', () => {
    const ids = optionIds(settingsPage, "value={$settings?.language ?? 'system'}", 'onchange');
    expect(ids.sort()).toEqual(constantsWithPrefix('Language').map(String).sort());
  });

  it('offers every network mode Go accepts, and no other', () => {
    const ids = optionIds(networkCard, 'const modeOptions', 'const proxyTypeOptions');
    expect(ids.sort()).toEqual(constantsWithPrefix('Network').map(String).sort());
  });

  it('offers every proxy type Go accepts, and no other', () => {
    const ids = optionIds(networkCard, 'const proxyTypeOptions', 'const interfaceOptions');
    expect(ids.sort()).toEqual(constantsWithPrefix('Proxy').map(String).sort());
  });

  it('offers the overlay hotkeys Go accepts, in the same order', () => {
    const wanted = ['OverlayHotkeyAltBacktick', 'OverlayHotkeyShiftF1', 'OverlayHotkeyShiftF2', 'OverlayHotkeyCtrlShiftO'];
    const constants = goConstants();
    expect([...OVERLAY_HOTKEYS]).toEqual(wanted.map((name) => constants.get(name)));
    expect(DEFAULT_OVERLAY_HOTKEY).toBe(constants.get('OverlayHotkeyAltBacktick'));
  });

  it('knows every presence status Go accepts, in the same order', () => {
    const constants = goConstants();
    const wanted = ['PresenceOnline', 'PresenceAway', 'PresenceBusy', 'PresenceInvisible'].map((name) => constants.get(name));
    expect([...PRESENCE_STATUSES]).toEqual(wanted);
    expect(DEFAULT_PRESENCE).toBe(constants.get('PresenceOnline'));
  });
});
