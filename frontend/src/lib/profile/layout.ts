import {
  BLOCK_TYPES,
  BLOCK_WIDTHS,
  COLLECTION_SOURCES,
  MAX_BLOCKS,
  MAX_BLOCK_TITLE,
  MAX_CAPTION,
  MAX_COLLECTION_GAMES,
  MAX_TEXT_BLOCKS,
  MAX_TEXT_BODY,
  type BlockType,
  type BlockWidth,
  type ProfileBlock,
  type ProfileLayout,
  type ProfileSettings,
} from '../services/account';
import { msg } from '../i18n';
import type { ProfileKey } from '../i18n/catalog/ru/profile';

export type LayoutErrorCode =
  | 'unknown_type'
  | 'not_found'
  | 'out_of_range'
  | 'invalid_width'
  | 'max_blocks'
  | 'max_text'
  | 'duplicate'
  | 'duplicate_source'
  | 'invalid_config'
  | 'incomplete'
  | 'bad_chars';

export interface LayoutError {
  code: LayoutErrorCode;
  field?: string;
}

export type LayoutResult = { ok: true; layout: ProfileLayout } | { ok: false; error: LayoutError };
export type AddResult = { ok: true; layout: ProfileLayout; id: string } | { ok: false; error: LayoutError };

export interface LayoutIssue {
  id: string;
  error: LayoutError;
}

export interface AddPreset {
  id: string;
  type: BlockType;
  width: BlockWidth;
  config: Record<string, unknown>;
}

type Config = Record<string, unknown>;
type VisibilityFlags = Pick<ProfileSettings, 'showPlaying' | 'showLibrary' | 'showActivity' | 'showStats'>;

const ONCE: ReadonlySet<string> = new Set(['genres', 'fingerprint', 'recent', 'activity', 'stats', 'playing', 'about']);

export const ADD_PRESETS: readonly AddPreset[] = [
  { id: 'pinned', type: 'pinned', width: 'full', config: { igdbId: 0, caption: '' } },
  { id: 'collection', type: 'collection', width: 'full', config: { source: 'manual', title: '', igdbIds: [] } },
  { id: 'favorites', type: 'collection', width: 'full', config: { source: 'favorites' } },
  { id: 'recently_completed', type: 'collection', width: 'full', config: { source: 'recently_completed' } },
  { id: 'most_played', type: 'collection', width: 'full', config: { source: 'most_played' } },
  { id: 'genres', type: 'genres', width: 'half', config: {} },
  { id: 'fingerprint', type: 'fingerprint', width: 'third', config: {} },
  { id: 'text', type: 'text', width: 'half', config: { title: '', body: '' } },
  { id: 'recent', type: 'recent', width: 'full', config: {} },
  { id: 'activity', type: 'activity', width: 'half', config: {} },
  { id: 'stats', type: 'stats', width: 'half', config: {} },
  { id: 'playing', type: 'playing', width: 'full', config: {} },
  { id: 'about', type: 'about', width: 'full', config: {} },
];

const ERROR_KEYS: Record<LayoutErrorCode, ProfileKey> = {
  unknown_type: 'profile.layoutErrUnknownType',
  not_found: 'profile.layoutErrNotFound',
  out_of_range: 'profile.layoutErrRange',
  invalid_width: 'profile.layoutErrWidth',
  max_blocks: 'profile.layoutErrMaxBlocks',
  max_text: 'profile.layoutErrMaxText',
  duplicate: 'profile.layoutErrDuplicate',
  duplicate_source: 'profile.layoutErrDuplicateSource',
  invalid_config: 'profile.layoutErrConfig',
  incomplete: 'profile.layoutErrIncomplete',
  bad_chars: 'profile.layoutErrChars',
};

export function layoutErrorText(error: LayoutError): string {
  if (error.code === 'max_blocks') return msg(ERROR_KEYS.max_blocks, { count: MAX_BLOCKS });
  if (error.code === 'max_text') return msg(ERROR_KEYS.max_text, { count: MAX_TEXT_BLOCKS });
  return msg(ERROR_KEYS[error.code]);
}

export function hasRejectedRune(value: string, multiline: boolean): boolean {
  const text = multiline ? value.replace(/\r\n/g, '\n') : value;
  for (const ch of text) {
    if (multiline && ch === '\n') continue;
    const code = ch.codePointAt(0) ?? 0;
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) return true;
    if (code === 0x2028 || code === 0x2029) return true;
    if ((code >= 0x202a && code <= 0x202e) || (code >= 0x2066 && code <= 0x2069)) return true;
  }
  return false;
}

export function statusIssue(text: string | undefined): LayoutError | null {
  return hasRejectedRune(text ?? '', false) ? { code: 'bad_chars', field: 'statusText' } : null;
}

export function unknownBlockCount(layout: ProfileLayout): number {
  return layout.blocks.filter((block) => !isKnownType(block.type)).length;
}

export function isKnownType(type: string): type is BlockType {
  return (BLOCK_TYPES as readonly string[]).includes(type);
}

export function runeCount(value: string): number {
  return [...value].length;
}

function fail(code: LayoutErrorCode, field?: string): { ok: false; error: LayoutError } {
  return { ok: false, error: field ? { code, field } : { code } };
}

export function defaultLayout(settings: Pick<ProfileSettings, 'showcase'>): ProfileLayout {
  const blocks: ProfileBlock[] = [{ id: 'playing', type: 'playing', width: 'full', config: {} }];
  settings.showcase.forEach((source, index) => {
    blocks.push({ id: `col${index + 1}`, type: 'collection', width: 'full', config: { source } });
  });
  blocks.push(
    { id: 'recent', type: 'recent', width: 'full', config: {} },
    { id: 'activity', type: 'activity', width: 'full', config: {} },
  );
  return { version: 1, blocks };
}

export function effectiveLayout(settings: Pick<ProfileSettings, 'showcase' | 'layout'>): ProfileLayout {
  return settings.layout ?? defaultLayout(settings);
}

export function newBlockId(layout: ProfileLayout): string {
  const taken = new Set(layout.blocks.map((block) => block.id));
  for (let n = 1; ; n++) {
    const id = `b${n.toString(36)}`;
    if (!taken.has(id)) return id;
  }
}

function sourceOf(config: Config): string {
  return typeof config.source === 'string' ? config.source : '';
}

function autoSourceOf(block: { type: string; config: Config }): string {
  if (block.type !== 'collection') return '';
  const source = sourceOf(block.config);
  return source && source !== 'manual' ? source : '';
}

function ids(config: Config): unknown[] {
  return Array.isArray(config.igdbIds) ? config.igdbIds : [];
}

export function configError(type: string, config: Config): LayoutError | null {
  if (typeof config !== 'object' || config === null || Array.isArray(config)) return { code: 'invalid_config' };
  if (type === 'pinned') {
    const igdbId = config.igdbId;
    if (igdbId !== undefined && (typeof igdbId !== 'number' || !Number.isInteger(igdbId) || igdbId < 0)) {
      return { code: 'invalid_config', field: 'igdbId' };
    }
    const caption = config.caption;
    if (caption !== undefined && (typeof caption !== 'string' || runeCount(caption) > MAX_CAPTION)) {
      return { code: 'invalid_config', field: 'caption' };
    }
  }
  if (type === 'collection') {
    const source = sourceOf(config);
    if (!(COLLECTION_SOURCES as readonly string[]).includes(source)) return { code: 'invalid_config', field: 'source' };
    const title = config.title;
    if (source === 'manual') {
      if (title !== undefined && (typeof title !== 'string' || runeCount(title) > MAX_BLOCK_TITLE)) {
        return { code: 'invalid_config', field: 'title' };
      }
      const list = ids(config);
      const unique = new Set(list);
      const bad = list.some((id) => typeof id !== 'number' || !Number.isInteger(id) || id <= 0);
      if (config.igdbIds !== undefined && !Array.isArray(config.igdbIds)) return { code: 'invalid_config', field: 'igdbIds' };
      if (bad || unique.size !== list.length || list.length > MAX_COLLECTION_GAMES) {
        return { code: 'invalid_config', field: 'igdbIds' };
      }
    } else {
      if (typeof title === 'string' && title !== '') return { code: 'invalid_config', field: 'title' };
      if (ids(config).length > 0) return { code: 'invalid_config', field: 'igdbIds' };
    }
  }
  if (type === 'text') {
    const title = config.title;
    if (title !== undefined && (typeof title !== 'string' || runeCount(title) > MAX_BLOCK_TITLE)) {
      return { code: 'invalid_config', field: 'title' };
    }
    const body = config.body;
    if (body !== undefined && (typeof body !== 'string' || runeCount(body) > MAX_TEXT_BODY)) {
      return { code: 'invalid_config', field: 'body' };
    }
  }
  return null;
}

function addError(layout: ProfileLayout, type: string, config: Config, ignoreId = ''): LayoutError | null {
  if (!isKnownType(type)) return { code: 'unknown_type' };
  const others = layout.blocks.filter((block) => block.id !== ignoreId);
  if (!ignoreId && layout.blocks.length >= MAX_BLOCKS) return { code: 'max_blocks' };
  if (ONCE.has(type) && others.some((block) => block.type === type)) return { code: 'duplicate' };
  if (type === 'text' && !ignoreId && others.filter((block) => block.type === 'text').length >= MAX_TEXT_BLOCKS) {
    return { code: 'max_text' };
  }
  const invalid = configError(type, config);
  if (invalid) return invalid;
  const source = autoSourceOf({ type, config });
  if (source && others.some((block) => autoSourceOf(block) === source)) return { code: 'duplicate_source', field: 'source' };
  return null;
}

export function canAdd(layout: ProfileLayout, preset: AddPreset): LayoutError | null {
  return addError(layout, preset.type, preset.config);
}

export function addBlock(
  layout: ProfileLayout,
  type: string,
  options: { width?: BlockWidth; config?: Config } = {},
): AddResult {
  const preset = ADD_PRESETS.find((item) => item.type === type);
  const config = options.config ?? preset?.config ?? {};
  const invalid = addError(layout, type, config);
  if (invalid) return { ok: false, error: invalid };
  const width = options.width ?? preset?.width ?? 'full';
  if (!(BLOCK_WIDTHS as readonly string[]).includes(width)) return fail('invalid_width');
  const id = newBlockId(layout);
  const block: ProfileBlock = { id, type, width, config: structuredCopy(config) };
  return { ok: true, layout: { ...layout, blocks: [...layout.blocks, block] }, id };
}

function structuredCopy<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function find(layout: ProfileLayout, id: string): { index: number; block: ProfileBlock } | null {
  const index = layout.blocks.findIndex((block) => block.id === id);
  return index < 0 ? null : { index, block: layout.blocks[index] };
}

export function removeBlock(layout: ProfileLayout, id: string): LayoutResult {
  const found = find(layout, id);
  if (!found) return fail('not_found');
  if (!isKnownType(found.block.type)) return fail('unknown_type');
  return { ok: true, layout: { ...layout, blocks: layout.blocks.filter((block) => block.id !== id) } };
}

export function moveBlock(layout: ProfileLayout, from: number, to: number): LayoutResult {
  const size = layout.blocks.length;
  const valid = (n: number) => Number.isInteger(n) && n >= 0 && n < size;
  if (!valid(from) || !valid(to)) return fail('out_of_range');
  if (from === to) return { ok: true, layout };
  const blocks = [...layout.blocks];
  const [moved] = blocks.splice(from, 1);
  blocks.splice(to, 0, moved);
  return { ok: true, layout: { ...layout, blocks } };
}

export function setWidth(layout: ProfileLayout, id: string, width: string): LayoutResult {
  const found = find(layout, id);
  if (!found) return fail('not_found');
  if (!isKnownType(found.block.type)) return fail('unknown_type');
  if (!(BLOCK_WIDTHS as readonly string[]).includes(width)) return fail('invalid_width');
  const blocks = layout.blocks.map((block) => (block.id === id ? { ...block, width: width as BlockWidth } : block));
  return { ok: true, layout: { ...layout, blocks } };
}

export function updateConfig(layout: ProfileLayout, id: string, patch: Config): LayoutResult {
  const found = find(layout, id);
  if (!found) return fail('not_found');
  const { block } = found;
  if (!isKnownType(block.type)) return fail('unknown_type');
  const config: Config = { ...block.config };
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined) delete config[key];
    else config[key] = value;
  }
  const invalid = addError(layout, block.type, config, id);
  if (invalid) return { ok: false, error: invalid };
  const blocks = layout.blocks.map((item) => (item.id === id ? { ...item, config } : item));
  return { ok: true, layout: { ...layout, blocks } };
}

export function blockIssue(block: ProfileBlock): LayoutError | null {
  if (!isKnownType(block.type)) return null;
  const invalid = configError(block.type, block.config);
  if (invalid) return invalid;
  const config = block.config;
  const lines: [string, boolean][] = [
    ['caption', false],
    ['title', false],
    ['body', true],
  ];
  if (block.type === 'pinned' || block.type === 'collection' || block.type === 'text') {
    for (const [field, multiline] of lines) {
      const value = config[field];
      if (typeof value === 'string' && hasRejectedRune(value, multiline)) return { code: 'bad_chars', field };
    }
  }
  if (block.type === 'pinned' && !(typeof config.igdbId === 'number' && config.igdbId > 0)) {
    return { code: 'incomplete', field: 'igdbId' };
  }
  if (block.type === 'collection' && sourceOf(config) === 'manual') {
    const title = typeof config.title === 'string' ? config.title.trim() : '';
    if (title === '') return { code: 'incomplete', field: 'title' };
    if (ids(config).length === 0) return { code: 'incomplete', field: 'igdbIds' };
  }
  if (block.type === 'text') {
    const body = typeof config.body === 'string' ? config.body.trim() : '';
    if (body === '') return { code: 'incomplete', field: 'body' };
  }
  return null;
}

export function validateLayout(layout: ProfileLayout): LayoutIssue[] {
  const issues: LayoutIssue[] = [];
  const seen = new Map<string, number>();
  let texts = 0;
  layout.blocks.forEach((block, index) => {
    const own = blockIssue(block);
    if (own) issues.push({ id: block.id, error: own });
    if (!isKnownType(block.type)) return;
    if (ONCE.has(block.type) && seen.has(block.type)) issues.push({ id: block.id, error: { code: 'duplicate' } });
    seen.set(block.type, index);
    if (block.type === 'text' && ++texts > MAX_TEXT_BLOCKS) issues.push({ id: block.id, error: { code: 'max_text' } });
    const source = autoSourceOf(block);
    if (source) {
      const key = `collection:${source}`;
      if (seen.has(key)) issues.push({ id: block.id, error: { code: 'duplicate_source', field: 'source' } });
      seen.set(key, index);
    }
  });
  if (layout.blocks.length > MAX_BLOCKS) {
    issues.push({ id: layout.blocks[MAX_BLOCKS].id, error: { code: 'max_blocks' } });
  }
  return issues;
}

export function hiddenFromOthers(type: string, flags: VisibilityFlags): boolean {
  switch (type) {
    case 'activity':
      return !flags.showActivity;
    case 'stats':
    case 'fingerprint':
    case 'genres':
      return !flags.showStats;
    case 'playing':
      return !flags.showPlaying;
    case 'recent':
    case 'pinned':
    case 'collection':
      return !flags.showLibrary;
    default:
      return false;
  }
}

export function visibleBlocks(layout: ProfileLayout, flags: VisibilityFlags): ProfileBlock[] {
  return layout.blocks.filter((block) => !hiddenFromOthers(block.type, flags));
}

function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${canonical(item)}`).join(',')}}`;
  }
  return JSON.stringify(value) ?? 'null';
}

export function sameLayout(a: ProfileLayout | null | undefined, b: ProfileLayout | null | undefined): boolean {
  return canonical(a ?? null) === canonical(b ?? null);
}

export type LayoutPatch = { send: false } | { send: true; layout: ProfileLayout | null };

export function layoutPatch(
  saved: Pick<ProfileSettings, 'showcase' | 'layout'>,
  draft: ProfileLayout,
  reset: boolean,
): LayoutPatch {
  if (reset && sameLayout(draft, defaultLayout(saved))) {
    return saved.layout ? { send: true, layout: null } : { send: false };
  }
  if (sameLayout(draft, effectiveLayout(saved))) return { send: false };
  return { send: true, layout: draft };
}
