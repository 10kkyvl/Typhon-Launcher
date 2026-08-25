import type { MediaAsset, MetadataView } from '../services/metadata';

const blanks = new Set(['', '-', '--', '—', 'n/a', 'na', 'null', 'undefined', 'unknown', 'неизвестно']);

export function clean(value: unknown): string {
  if (value === null || value === undefined) return '';
  const text = String(value).trim();
  return blanks.has(text.toLowerCase()) ? '' : text;
}

export function landscape(asset: Pick<MediaAsset, 'width' | 'height'>): boolean {
  return asset.width > 0 && asset.height > 0 && asset.width > asset.height;
}

export function pickHero(hero: string, screenshots: MediaAsset[]): string {
  const direct = clean(hero);
  if (direct) return direct;
  for (const shot of screenshots) {
    const url = clean(shot.url);
    if (url && landscape(shot)) return url;
  }
  return '';
}

export function galleryShots(screenshots: MediaAsset[], heroSrc: string): MediaAsset[] {
  const hero = clean(heroSrc);
  return screenshots.filter((shot) => {
    const url = clean(shot.url);
    return url !== '' && url !== hero;
  });
}

function hasContent(view: MetadataView): boolean {
  return Boolean(
    clean(view.game.title) ||
      clean(view.game.summary) ||
      clean(view.cover) ||
      clean(view.hero) ||
      view.screenshots.length > 0,
  );
}

export function preferView(previous: MetadataView | null, next: MetadataView): MetadataView {
  if (!previous || previous.game.id !== next.game.id) return next;
  if (hasContent(next) || !hasContent(previous)) return next;
  return previous;
}

export interface MetaLineInput {
  year?: number | string | null;
  developer?: string | null;
  publisher?: string | null;
  genres?: string[] | null;
  platforms?: string[] | null;
}

export function metaLine(input: MetaLineInput): string[] {
  const parts: string[] = [];
  const push = (value: unknown) => {
    const text = clean(value);
    if (!text) return;
    if (parts.some((part) => part.toLowerCase() === text.toLowerCase())) return;
    parts.push(text);
  };
  push(input.year);
  push(clean(input.developer) || input.publisher);
  push(input.genres?.[0]);
  push(input.platforms?.[0]);
  return parts;
}

export interface Fact {
  label: string;
  value: string;
  mono?: boolean;
  full?: string;
}

export interface FactInput {
  label: string;
  value?: unknown;
  mono?: boolean;
  full?: unknown;
}

export function facts(list: FactInput[]): Fact[] {
  const out: Fact[] = [];
  for (const item of list) {
    const value = clean(item.value);
    if (!value) continue;
    const fact: Fact = { label: item.label, value };
    if (item.mono) fact.mono = true;
    const full = clean(item.full);
    if (full && full !== value) fact.full = full;
    out.push(fact);
  }
  return out;
}

export function tagList(groups: (string[] | null | undefined)[], limit = 10): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const group of groups) {
    for (const raw of group ?? []) {
      const tag = clean(raw);
      if (!tag) continue;
      const key = tag.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(tag);
      if (out.length >= limit) return out;
    }
  }
  return out;
}

const desktopPlatform = /windows|(^|[^a-z])pc([^a-z]|$)/i;

export function orderPlatforms(platforms: string[] | null | undefined): string[] {
  const list = tagList([platforms], 32);
  const index = list.findIndex((platform) => desktopPlatform.test(platform));
  if (index <= 0) return list;
  return [list[index], ...list.slice(0, index), ...list.slice(index + 1)];
}

export function joinLimited(list: string[], limit: number): string {
  if (list.length <= limit) return list.join(', ');
  return `${list.slice(0, limit).join(', ')} +${list.length - limit}`;
}

export interface SummaryView {
  text: string;
  expandable: boolean;
}

export function summaryView(raw: string, expanded: boolean, limit = 520): SummaryView {
  const text = clean(raw);
  if (!text) return { text: '', expandable: false };
  if (text.length <= limit) return { text, expandable: false };
  if (expanded) return { text, expandable: true };
  const cut = text.slice(0, limit);
  const space = cut.lastIndexOf(' ');
  const head = space > limit * 0.6 ? cut.slice(0, space) : cut;
  return { text: `${head.replace(/[\s.,;:—-]+$/, '')}…`, expandable: true };
}

export type PrimaryKind = 'play' | 'stop' | 'update' | 'install' | 'progress' | 'resolving' | 'unavailable';

export interface PrimaryAction {
  kind: PrimaryKind;
  label: string;
  disabled: boolean;
  progress?: number;
}

export interface BusyState {
  label: string;
  progress: number;
}

export interface GameStatus {
  installed: boolean;
  running: boolean;
  updateAvailable: boolean;
  releaseCount: number;
  releasesLoading: boolean;
  busy?: BusyState | null;
}

export function primaryAction(status: GameStatus): PrimaryAction {
  if (status.busy) {
    return { kind: 'progress', label: status.busy.label, disabled: true, progress: status.busy.progress };
  }
  if (status.running) return { kind: 'stop', label: 'Остановить', disabled: false };
  if (status.installed) {
    if (status.updateAvailable) return { kind: 'update', label: 'Обновить', disabled: false };
    return { kind: 'play', label: 'Играть', disabled: false };
  }
  if (status.releaseCount > 0) return { kind: 'install', label: 'Установить', disabled: false };
  if (status.releasesLoading) return { kind: 'resolving', label: 'Проверяем загрузки…', disabled: true };
  return { kind: 'unavailable', label: 'Нет доступных загрузок', disabled: true };
}

export function busyState(
  entries: (({ active: boolean; label: string; progress: number }) | null | undefined)[],
): BusyState | null {
  for (const entry of entries) {
    if (!entry || !entry.active) continue;
    const label = clean(entry.label);
    if (!label) continue;
    const progress = Number.isFinite(entry.progress) ? Math.min(1, Math.max(0, entry.progress)) : 0;
    return { label, progress };
  }
  return null;
}

export function stepIndex(index: number, length: number, delta: number): number {
  if (length <= 0) return 0;
  const base = Number.isFinite(index) ? Math.trunc(index) : 0;
  return (((base + delta) % length) + length) % length;
}

export function languageLabel(languages: string[] | undefined, limit = 3): string {
  const list = tagList([languages], 32);
  if (list.length === 0) return '';
  const shown = list.slice(0, limit).map((lang) => (lang.length <= 3 ? lang.toUpperCase() : lang));
  const rest = list.length - shown.length;
  return rest > 0 ? `${shown.join(', ')} +${rest}` : shown.join(', ');
}
