import { Service as SocialService } from '../../../bindings/typhon/internal/social';
import { AccountError, toAccountError } from './account';
import { inWails } from './backend';

export type Relation = 'self' | 'friend' | 'incoming' | 'outgoing' | 'none' | 'blocked';

export interface UserCard {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string;
}

export interface PresenceView {
  status: string;
  gameId?: number | null;
  gameTitle?: string;
  since?: string | null;
  lastSeenAt?: string | null;
}

export interface FriendView extends UserCard {
  since: string;
  presence?: PresenceView | null;
}

export interface RequestView extends UserCard {
  createdAt: string;
  mutualCount: number;
  commonCount: number;
}

export interface FriendsPage {
  friends: FriendView[];
  incoming: RequestView[];
  outgoing: RequestView[];
}

export interface RequestsSignal {
  incoming: number;
}

export interface SendResult {
  request: RequestView;
  accepted: boolean;
}

export interface GameCard {
  igdbId: number;
  title: string;
  coverUrl: string;
}

export interface PlayedGame extends GameCard {
  playtimeSeconds?: number | null;
  status: string;
  favorite: boolean;
  lastPlayedAt: string | null;
}

export interface CommonGame extends GameCard {
  viewerOwned: boolean;
  targetOwned: boolean;
}

export interface CommonGames {
  count: number;
  games: CommonGame[];
}

export interface ReactionCount {
  emoji: string;
  count: number;
}

export interface ActivityView {
  id: number;
  kind: string;
  game: GameCard;
  createdAt: string;
}

export interface FeedEvent {
  id: number;
  user: UserCard;
  kind: string;
  game: GameCard;
  createdAt: string;
  reactions: ReactionCount[];
  mine: string[];
}

export interface FeedPage {
  events: FeedEvent[];
  next: number;
}

export interface StatsView {
  games: number;
  completed: number;
  hours?: number | null;
}

export interface ShowcaseBlock {
  kind: string;
  games: GameCard[];
}

export interface PublicProfile extends UserCard {
  bio: string;
  relation: string;
  visibility: string;
  stats: StatsView | null;
  favorites: GameCard[];
  showcase: ShowcaseBlock[];
  recentlyPlayed: PlayedGame[];
  recentActivity: ActivityView[];
  common: CommonGames | null;
  mutualFriends: UserCard[];
  mutualCount: number;
  createdAt: string;
  presence?: PresenceView | null;
}

export interface GamesPage {
  games: PlayedGame[];
  next: string;
}

export interface GameFriend extends UserCard {
  playtimeSeconds?: number | null;
  status: string;
}

export interface GameFriends {
  played: GameFriend[];
  playingNow: UserCard[];
}

const unauthenticated = () => new AccountError('unauthenticated');

export function emptyFriendsPage(): FriendsPage {
  return { friends: [], incoming: [], outgoing: [] };
}

export function emptyFeedPage(): FeedPage {
  return { events: [], next: 0 };
}

export function emptyGameFriends(): GameFriends {
  return { played: [], playingNow: [] };
}

function list<T>(value: T[] | null | undefined): T[] {
  return value ?? [];
}

export function toFriendsPage(value: unknown): FriendsPage {
  const page = value as Partial<FriendsPage> | null;
  if (!page) return emptyFriendsPage();
  return {
    friends: list(page.friends),
    incoming: list(page.incoming),
    outgoing: list(page.outgoing),
  };
}

export async function friends(): Promise<FriendsPage> {
  if (!inWails) return emptyFriendsPage();
  try {
    return toFriendsPage(await SocialService.Friends());
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function refresh(): Promise<void> {
  if (!inWails) return;
  try {
    await SocialService.Refresh();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function kick(): Promise<void> {
  if (!inWails) return;
  try {
    await SocialService.Kick();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function sendRequest(query: string): Promise<SendResult> {
  if (!inWails) throw unauthenticated();
  try {
    const result = (await SocialService.SendRequest(query)) as unknown as SendResult | null;
    if (!result) throw new Error('server_error');
    return result;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function accept(userId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Accept(userId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function decline(userId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Decline(userId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function unfriend(userId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Unfriend(userId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function block(userId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Block(userId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function unblock(userId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Unblock(userId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function blocks(): Promise<UserCard[]> {
  if (!inWails) return [];
  try {
    return list((await SocialService.Blocks()) as unknown as UserCard[] | null);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function friendCode(): Promise<string> {
  if (!inWails) throw unauthenticated();
  try {
    return await SocialService.FriendCode();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function rotateFriendCode(): Promise<string> {
  if (!inWails) throw unauthenticated();
  try {
    return await SocialService.RotateFriendCode();
  } catch (err) {
    throw toAccountError(err);
  }
}

function toProfile(value: unknown): PublicProfile {
  const profile = value as (Partial<PublicProfile> & UserCard) | null;
  if (!profile) throw new Error('server_error');
  return {
    id: profile.id,
    username: profile.username,
    displayName: profile.displayName,
    avatarUrl: profile.avatarUrl,
    bio: profile.bio ?? '',
    relation: profile.relation ?? 'none',
    visibility: profile.visibility ?? '',
    stats: profile.stats ?? null,
    favorites: list(profile.favorites),
    showcase: list(profile.showcase),
    recentlyPlayed: list(profile.recentlyPlayed),
    recentActivity: list(profile.recentActivity),
    common: profile.common ?? null,
    mutualFriends: list(profile.mutualFriends),
    mutualCount: profile.mutualCount ?? 0,
    createdAt: profile.createdAt ?? '',
    presence: profile.presence ?? null,
  };
}

export async function profile(username: string): Promise<PublicProfile> {
  if (!inWails) throw unauthenticated();
  try {
    return toProfile(await SocialService.Profile(username));
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function profileByCode(code: string): Promise<PublicProfile> {
  if (!inWails) throw unauthenticated();
  try {
    return toProfile(await SocialService.ProfileByCode(code));
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function userGames(username: string, cursor = ''): Promise<GamesPage> {
  if (!inWails) return { games: [], next: '' };
  try {
    const page = (await SocialService.UserGames(username, cursor)) as unknown as Partial<GamesPage> | null;
    return { games: list(page?.games), next: page?.next ?? '' };
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function gameFriends(canonicalGameId: string): Promise<GameFriends> {
  if (!inWails) return emptyGameFriends();
  try {
    const page = (await SocialService.GameFriends(canonicalGameId)) as unknown as Partial<GameFriends> | null;
    return { played: list(page?.played), playingNow: list(page?.playingNow) };
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function feed(cursor = ''): Promise<FeedPage> {
  if (!inWails) return emptyFeedPage();
  try {
    const page = (await SocialService.Feed(cursor)) as unknown as Partial<FeedPage> | null;
    return {
      events: list(page?.events).map((event) => ({
        ...event,
        reactions: list(event.reactions),
        mine: list(event.mine),
      })),
      next: page?.next ?? 0,
    };
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function react(id: string, emoji: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.React(id, emoji);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function unreact(id: string, emoji: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await SocialService.Unreact(id, emoji);
  } catch (err) {
    throw toAccountError(err);
  }
}
