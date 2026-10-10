import { MAX_COLLECTION_GAMES, type AutoSource, type BlockWidth, type ProfileBlock, type ProfileLayout, type ShowcaseKind } from '../services/account';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';
import type { GameRef, ProfileSnapshot } from '../services/profile';
import type { GameCard, GenreBreakdown, PlayedGame, PublicBlock, PublicLayout, PublicProfile } from '../services/social';
import { msg } from '../i18n';
import { blockIssue, defaultLayout, hiddenFromOthers } from './layout';
import { coverOf, showcaseLabel } from './view';

export interface BlockGame {
  key: string;
  title: string;
  cover: string;
  hero: string;
  seconds?: number | null;
  status?: string;
  completedAt?: string | null;
  open: () => void;
}

export type BlockBody =
  | { kind: 'pinned'; game: BlockGame | null; caption: string }
  | { kind: 'collection'; title: string; games: BlockGame[]; dated: boolean; hearts: boolean }
  | { kind: 'genres'; breakdown: GenreBreakdown }
  | { kind: 'fingerprint'; breakdown: GenreBreakdown; hours: number; games: number; completed: number }
  | { kind: 'text'; title: string; body: string }
  | { kind: 'external'; empty: boolean }
  | { kind: 'error' }
  | { kind: 'unknown' };

export interface GridBlock {
  id: string;
  index: number;
  type: string;
  width: BlockWidth;
  hidden: boolean;
  issue: boolean;
  body: BlockBody;
}

type Flags = Parameters<typeof hiddenFromOthers>[1];

export interface OwnContext {
  flags: Flags;
  snapshot: ProfileSnapshot;
  preview: ProfileSnapshot | null;
  art: Record<string, GameArt>;
  picked: Record<number, GameRef>;
  open: (game: GameRef) => void;
  empty: (type: string) => boolean;
}

export interface PublicContext {
  profile: PublicProfile;
  open: (game: GameCard) => void;
  empty: (type: string) => boolean;
}

const EMPTY_BREAKDOWN: GenreBreakdown = { genres: [], other: 0, unknown: 0 };

function list<T>(value: readonly T[] | null | undefined): T[] {
  return Array.isArray(value) ? [...value] : [];
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function whole(value: unknown): number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : 0;
}

function breakdownOf(value: Partial<GenreBreakdown> | null | undefined): GenreBreakdown {
  if (!value) return EMPTY_BREAKDOWN;
  return { genres: list(value.genres), other: value.other ?? 0, unknown: value.unknown ?? 0 };
}

export function refOfLibrary(game: LibraryGame): GameRef {
  return {
    id: game.id,
    title: game.title,
    cover: game.cover,
    canonicalGameId: game.canonicalGameId,
    playtimeSeconds: game.playtimeSeconds,
    status: game.status ?? '',
    statusAt: game.statusAt ?? null,
  };
}

export function showcaseTitle(kind: string): string {
  return kind === 'favorites' ? msg('social.favoriteGamesTitle') : showcaseLabel(kind);
}

export function heroOf(game: GameRef, art: Record<string, GameArt>): string {
  return (game.canonicalGameId && art[game.canonicalGameId]?.hero) || '';
}

function ownGame(ref: GameRef, key: string, ctx: OwnContext): BlockGame {
  return {
    key,
    title: ref.title,
    cover: coverOf(ref, ctx.art),
    hero: heroOf(ref, ctx.art),
    seconds: ref.playtimeSeconds,
    status: ref.status,
    completedAt: ref.status === 'completed' ? (ref.statusAt ?? null) : null,
    open: () => ctx.open(ref),
  };
}

function lookup(ctx: OwnContext, igdbId: number): { game: GameRef | null; failed: boolean } {
  const picked = ctx.picked[igdbId];
  if (picked) return { game: picked, failed: false };
  const entry = ctx.snapshot.layoutGames?.[String(igdbId)];
  if (entry?.game) return { game: entry.game, failed: false };
  return { game: null, failed: Boolean(entry?.error) };
}

function ownBody(block: ProfileBlock, ctx: OwnContext): BlockBody {
  const config = block.config ?? {};
  switch (block.type) {
    case 'pinned': {
      const id = whole(config.igdbId);
      const caption = text(config.caption);
      if (id === 0) return { kind: 'pinned', game: null, caption };
      const found = lookup(ctx, id);
      if (found.failed) return { kind: 'error' };
      return { kind: 'pinned', game: found.game ? ownGame(found.game, String(id), ctx) : null, caption };
    }
    case 'collection': {
      const source = text(config.source);
      if (source !== 'manual') {
        const shelf = (ctx.preview ?? ctx.snapshot).showcase.find((item) => item.kind === source);
        const games = list(shelf?.games)
          .slice(0, MAX_COLLECTION_GAMES)
          .map((ref) => ownGame(ref, ref.id, ctx));
        return { kind: 'collection', title: showcaseTitle(source), games, dated: source === 'recently_completed', hearts: source === 'favorites' };
      }
      const ids: unknown[] = Array.isArray(config.igdbIds) ? config.igdbIds : [];
      const found = ids.map((raw) => {
        const id = whole(raw);
        return { id, ...lookup(ctx, id) };
      });
      const games = found.flatMap((item) => (item.game ? [ownGame(item.game, String(item.id), ctx)] : []));
      if (games.length === 0 && found.some((item) => item.failed)) return { kind: 'error' };
      return { kind: 'collection', title: text(config.title), games, dated: false, hearts: false };
    }
    case 'genres':
      return { kind: 'genres', breakdown: breakdownOf(ctx.snapshot.genres) };
    case 'fingerprint': {
      const { hours, games, completed } = ctx.snapshot.fingerprint;
      return { kind: 'fingerprint', breakdown: breakdownOf(ctx.snapshot.genres), hours, games, completed };
    }
    case 'text':
      return { kind: 'text', title: text(config.title), body: text(config.body) };
    case 'recent':
    case 'activity':
    case 'stats':
    case 'playing':
    case 'about':
      return { kind: 'external', empty: ctx.empty(block.type) };
    default:
      return { kind: 'unknown' };
  }
}

export function resolveOwn(layout: ProfileLayout, ctx: OwnContext): GridBlock[] {
  return layout.blocks.map((block, index) => ({
    id: block.id,
    index,
    type: block.type,
    width: block.width,
    hidden: hiddenFromOthers(block.type, ctx.flags),
    issue: blockIssue(block) !== null,
    body: ownBody(block, ctx),
  }));
}

function publicGame(card: GameCard, ctx: PublicContext): BlockGame {
  const played = card as Partial<PlayedGame> & { completedAt?: string | null };
  return {
    key: String(card.igdbId),
    title: card.title,
    cover: card.coverUrl,
    hero: card.heroUrl ?? '',
    seconds: played.playtimeSeconds,
    status: played.status,
    completedAt: played.completedAt ?? null,
    open: () => ctx.open(card),
  };
}

function publicBody(block: PublicBlock, ctx: PublicContext): BlockBody {
  const config = block.config ?? {};
  const data = block.data;
  switch (block.type) {
    case 'pinned':
      if (!data || data.error) return { kind: 'error' };
      return {
        kind: 'pinned',
        game: data.game ? publicGame(data.game, ctx) : null,
        caption: text(config.caption),
      };
    case 'collection': {
      const source = text(config.source);
      if (data?.error) return { kind: 'error' };
      if (!data && source === 'manual') return { kind: 'error' };
      const cards = data ? list(data.games) : list(ctx.profile.showcase.find((item) => item.kind === source)?.games);
      const title = data?.title || (source === 'manual' ? text(config.title) : showcaseTitle(source));
      return {
        kind: 'collection',
        title,
        games: cards.slice(0, MAX_COLLECTION_GAMES).map((card) => publicGame(card, ctx)),
        dated: source === 'recently_completed',
        hearts: source === 'favorites',
      };
    }
    case 'genres':
      if (!data || data.error) return { kind: 'error' };
      return { kind: 'genres', breakdown: breakdownOf({ genres: data.genres, other: data.other, unknown: data.unknown }) };
    case 'fingerprint': {
      if (!data || data.error) return { kind: 'error' };
      const stats = ctx.profile.stats;
      const count: unknown = data.games;
      return {
        kind: 'fingerprint',
        breakdown: breakdownOf({ genres: data.genres, other: data.other, unknown: data.unknown }),
        hours: data.hours ?? stats?.hours ?? 0,
        games: typeof count === 'number' ? count : (stats?.games ?? 0),
        completed: data.completed ?? stats?.completed ?? 0,
      };
    }
    case 'text':
      return { kind: 'text', title: text(config.title), body: text(config.body) };
    case 'recent':
    case 'activity':
    case 'stats':
    case 'playing':
    case 'about':
      return { kind: 'external', empty: ctx.empty(block.type) };
    default:
      return { kind: 'unknown' };
  }
}

export function publicLayout(profile: PublicProfile): PublicLayout {
  if (profile.layout) return profile.layout;
  const showcase = list(profile.showcase).map((block) => block.kind as ShowcaseKind);
  const blocks = defaultLayout({ showcase })
    .blocks.filter((block) => block.type !== 'stats' && block.type !== 'about')
    .map((block) => (block.type === 'activity' ? { ...block, width: 'full' as const } : block));
  return { version: 1, blocks };
}

export function resolvePublic(layout: PublicLayout, ctx: PublicContext): GridBlock[] {
  return layout.blocks.map((block, index) => ({
    id: block.id,
    index,
    type: block.type,
    width: block.width,
    hidden: false,
    issue: false,
    body: publicBody(block, ctx),
  }));
}

export function ownAutoArt(
  source: AutoSource,
  from: { running: GameRef[]; mostPlayed: GameRef | undefined; blocks: GridBlock[]; art: Record<string, GameArt> },
): string {
  if (source === 'playing') {
    const game = from.running[0];
    return game ? heroOf(game, from.art) || coverOf(game, from.art) : '';
  }
  if (source === 'most_played') {
    const game = from.mostPlayed;
    return game ? heroOf(game, from.art) || coverOf(game, from.art) : '';
  }
  for (const block of from.blocks) {
    if (block.body.kind === 'pinned' && block.body.game) return block.body.game.hero || block.body.game.cover;
  }
  return '';
}
