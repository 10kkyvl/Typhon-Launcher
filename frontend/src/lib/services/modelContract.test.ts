import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));

import { getSettings } from './settings';
import { bindingSources, frontendSources } from '../testing/sources';

interface Field {
  name: string;
  optional: boolean;
  type: string;
}

type Primitive = 'string' | 'number' | 'boolean';

function generatedModels(): Map<string, Map<string, Field>> {
  const models = new Map<string, Map<string, Field>>();
  for (const [path, text] of Object.entries(bindingSources)) {
    const pkg = /^typhon\/internal\/(\w+)\/models\.ts$/.exec(path)?.[1];
    if (!pkg) continue;
    for (const match of text.matchAll(/export interface (\w+) \{([\s\S]*?)\n\}/g)) {
      const fields = new Map<string, Field>();
      for (const field of match[2].matchAll(/^\s*"(\w+)"(\??):\s*([^;]+);/gm)) {
        fields.set(field[1], { name: field[1], optional: field[2] === '?', type: field[3].trim() });
      }
      models.set(`${pkg}.${match[1]}`, fields);
    }
  }
  return models;
}

interface Declared {
  file: string;
  name: string;
  parents: string[];
  fields: Map<string, Field>;
}

function declaredInterfaces(): Declared[] {
  const found: Declared[] = [];
  for (const [file, text] of Object.entries(frontendSources)) {
    if (!/bindings\/typhon\/internal\//.test(text)) continue;
    for (const match of text.matchAll(/export interface (\w+)(?:\s+extends\s+([^{]+))?\s*\{([\s\S]*?)\n\}/g)) {
      const fields = new Map<string, Field>();
      for (const field of match[3].matchAll(/^ {2}(\w+)(\??):\s*([^;]+);/gm)) {
        fields.set(field[1], { name: field[1], optional: field[2] === '?', type: field[3].trim() });
      }
      const parents = (match[2] ?? '').split(',').map((part) => part.trim().replace(/<.*$/, '')).filter(Boolean);
      found.push({ file, name: match[1], parents, fields });
    }
  }
  return found;
}

function importedPackages(file: string): string[] {
  return [...frontendSources[file].matchAll(/bindings\/typhon\/internal\/(\w+)/g)].map((match) => match[1]);
}

function ownAndInherited(declared: Declared, all: Declared[]): Map<string, Field> | null {
  const fields = new Map(declared.fields);
  for (const parent of declared.parents) {
    const base = all.find((candidate) => candidate.file === declared.file && candidate.name === parent);
    if (!base) return null;
    const inherited = ownAndInherited(base, all);
    if (!inherited) return null;
    for (const [name, field] of inherited) fields.set(name, field);
  }
  return fields;
}

function primitive(type: string): Primitive | null {
  const base = type
    .replace(/\$models\./g, '')
    .replace(/\|\s*null\b/g, '')
    .replace(/\bnull\s*\|/g, '')
    .trim();
  if (base === 'string' || /^(?:['"][^'"]*['"])(?:\s*\|\s*['"][^'"]*['"])*$/.test(base)) return 'string';
  if (base === 'number') return 'number';
  if (base === 'boolean') return 'boolean';
  return null;
}

interface Pair {
  where: string;
  declared: Map<string, Field>;
  model: Map<string, Field>;
}

// Interface types whose Go struct carries another name.
const GO_NAMES: Record<string, string> = {
  'metadata.MetadataView': 'metadata.View',
  'metadata.GameArt': 'metadata.Art',
  'relocate.MoveJob': 'relocate.Job',
};

// Interfaces a screen reads whole: a field Go sends and the interface lacks is one the screen cannot
// show (the games sharing a folder, a move left without its card). Fields left out on purpose are the value.
const FULL_MIRRORS: Record<string, string[]> = {
  'lib/services/install.ts: RemovalInfo (install)': [],
  'lib/services/relocate.ts: MoveJob (relocate)': [],
  'lib/services/diskusage.ts: GameUsage (diskusage)': [],
  'lib/services/diskusage.ts: DownloadUsage (diskusage)': [],
  'lib/services/diskusage.ts: Leftover (diskusage)': [],
  'lib/services/diskusage.ts: GroupUsage (diskusage)': [],
  'lib/services/diskusage.ts: GroupDelete (diskusage)': [],
  'lib/services/diskusage.ts: Warning (diskusage)': [],
  'lib/services/diskusage.ts: Totals (diskusage)': [],
  'lib/services/diskusage.ts: Report (diskusage)': [],
};

// Fields the interface declares required while Go tags them omitempty: a reader trusting the type
// meets undefined. Shrink, never grow.
const REQUIRED_THOUGH_OMITTED = new Set<string>([
  'lib/services/account.ts: ProfileSettings (account).showcase',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).compatOnly',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).defaultSort',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).genre',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).hideLibrary',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).kind',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).notInterested',
  'lib/services/recommendations.ts: RecommendationPreferences (catalog).platform',
  'lib/services/social.ts: ActivityView (social).achievements',
  'lib/services/social.ts: ActivityView (social).review',
  'lib/services/social.ts: ActivityView (social).seconds',
]);

function pairs(): Pair[] {
  const models = generatedModels();
  const all = declaredInterfaces();
  const out: Pair[] = [];
  for (const declared of all) {
    const fields = ownAndInherited(declared, all);
    if (!fields) continue;
    for (const pkg of new Set(importedPackages(declared.file))) {
      const key = `${pkg}.${declared.name}`;
      const model = models.get(GO_NAMES[key] ?? key);
      if (model) out.push({ where: `${declared.file}: ${declared.name} (${pkg})`, declared: fields, model });
    }
  }
  return out;
}

describe('hand-written backend types', () => {
  const compared = pairs();

  it('are matched against a good share of the generated models', () => {
    expect(compared.length).toBeGreaterThan(60);
  });

  it('pairs the metadata view and art with their Go models', () => {
    const art = compared.find((entry) => entry.where.includes('GameArt (metadata)'));
    const view = compared.find((entry) => entry.where.includes('MetadataView (metadata)'));
    expect(art?.model.get('heroIsPoster')).toMatchObject({ type: 'boolean', optional: false });
    expect(view?.model.get('heroIsPoster')).toMatchObject({ type: 'boolean', optional: false });
  });

  it('declare no field the Go struct does not send', () => {
    const phantom = compared.flatMap(({ where, declared, model }) =>
      [...declared.keys()].filter((name) => !model.has(name)).map((name) => `${where}.${name}`),
    );
    expect(phantom).toEqual([]);
  });

  it('leave out no field of the Go struct where a screen reads the interface whole', () => {
    const found = compared.filter(({ where }) => where in FULL_MIRRORS);
    expect(found.map(({ where }) => where).sort()).toEqual(Object.keys(FULL_MIRRORS).sort());
    const dropped = found.flatMap(({ where, declared, model }) =>
      [...model.keys()].filter((name) => !declared.has(name) && !FULL_MIRRORS[where].includes(name)).map((name) => `${where}.${name}`),
    );
    expect(dropped).toEqual([]);
  });

  it('agree with the Go struct on whether a plain field is text, a number or a flag', () => {
    const clashes: string[] = [];
    for (const { where, declared, model } of compared) {
      for (const [name, field] of declared) {
        const generated = model.get(name);
        if (!generated) continue;
        const left = primitive(generated.type);
        const right = primitive(field.type);
        if (left && right && left !== right) {
          clashes.push(`${where}.${name}: Go sends ${generated.type}, interface says ${field.type}`);
        }
      }
    }
    expect(clashes).toEqual([]);
  });

  it('mark optional every field Go may leave out', () => {
    const required: string[] = [];
    for (const { where, declared, model } of compared) {
      for (const [name, field] of declared) {
        if (model.get(name)?.optional && !field.optional) required.push(`${where}.${name}`);
      }
    }
    expect(required.filter((entry) => !REQUIRED_THOUGH_OMITTED.has(entry))).toEqual([]);
    expect([...REQUIRED_THOUGH_OMITTED].filter((entry) => !required.includes(entry)), 'fixed: drop it from the list').toEqual([]);
  });
});

describe('settings mirror', () => {
  const model = generatedModels().get('settings.Settings');

  it('finds the generated settings model', () => {
    expect(model).toBeDefined();
    expect(model!.size).toBeGreaterThan(40);
  });

  it('gives the browser fallback exactly the fields the Go settings carry', async () => {
    const defaults = await getSettings();
    const mine = Object.keys(defaults).sort();
    const theirs = [...model!.keys()].sort();
    expect(mine).toEqual(theirs);
  });

  it('gives each fallback default the type the Go field has', async () => {
    const defaults = (await getSettings()) as unknown as Record<string, unknown>;
    const wrong = [...model!.values()]
      .filter((field) => primitive(field.type) && typeof defaults[field.name] !== primitive(field.type))
      .map((field) => `${field.name}: Go ${field.type}, default ${typeof defaults[field.name]}`);
    expect(wrong).toEqual([]);
  });

  it('declares the same fields in the interface as in the fallback', () => {
    const declared = declaredInterfaces().find((entry) => entry.file === 'lib/services/settings.ts' && entry.name === 'Settings');
    expect(declared).toBeDefined();
    expect([...declared!.fields.keys()].sort()).toEqual([...model!.keys()].sort());
  });
});
