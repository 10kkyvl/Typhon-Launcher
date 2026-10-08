import { beforeEach, describe, expect, it, vi } from 'vitest';
import { answer, calls, resetBindings } from '../testing/fakeBinding';

const FAKE = '../testing/fakeBinding';

vi.mock('../../../bindings/typhon/internal/social', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('social'),
}));

type Fn = (...args: unknown[]) => Promise<unknown>;
type Social = Record<string, Fn>;

async function load(inWails: boolean): Promise<{ social: Social; AccountError: new (code: string) => Error & { code: string } }> {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  const social = (await import('./social')) as unknown as Social;
  const account = await import('./account');
  return { social, AccountError: account.AccountError };
}

beforeEach(() => {
  resetBindings();
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

describe('friends list', () => {
  it('is empty without a backend', async () => {
    const { social } = await load(false);

    await expect(social.friends()).resolves.toEqual({ friends: [], incoming: [], outgoing: [] });
    expect(calls).toEqual([]);
  });

  it('turns every list Go sent as null into an empty one', async () => {
    answer('social.Friends', { friends: null, incoming: [{ id: 'r1' }], outgoing: undefined });
    const { social } = await load(true);

    await expect(social.friends()).resolves.toEqual({ friends: [], incoming: [{ id: 'r1' }], outgoing: [] });
  });

  it('turns a missing page into an empty one', async () => {
    answer('social.Friends', null);
    const { social } = await load(true);

    await expect(social.friends()).resolves.toEqual({ friends: [], incoming: [], outgoing: [] });
  });

  it('exposes the conversion for pushed events', async () => {
    const { social } = await load(true);

    expect(social.toFriendsPage(null)).toEqual({ friends: [], incoming: [], outgoing: [] });
    expect(social.toFriendsPage({ friends: [{ id: 'a' }] })).toEqual({ friends: [{ id: 'a' }], incoming: [], outgoing: [] });
  });
});

describe('account errors', () => {
  it('keeps a code the interface knows, so the right message is shown', async () => {
    answer('social.SendRequest', () => {
      throw new Error('friend_limit');
    });
    const { social, AccountError } = await load(true);

    const error = await social.sendRequest('someone').catch((err: unknown) => err);

    expect(error).toBeInstanceOf(AccountError);
    expect((error as { code: string }).code).toBe('friend_limit');
  });

  it('turns an unexpected failure into a server error instead of showing its text', async () => {
    answer('social.Accept', () => {
      throw new Error('connection reset by peer');
    });
    const { social } = await load(true);

    const error = await social.accept('u1').catch((err: unknown) => err);

    expect((error as { code: string }).code).toBe('server_error');
    expect((error as Error).message).not.toContain('connection reset');
  });

  it('reports an empty reply to a friend request as a server error', async () => {
    answer('social.SendRequest', null);
    const { social } = await load(true);

    const error = await social.sendRequest('someone').catch((err: unknown) => err);

    expect((error as { code: string }).code).toBe('server_error');
  });

  it('hands a successful friend request result over as it is', async () => {
    answer('social.SendRequest', { status: 'sent' });
    const { social } = await load(true);

    await expect(social.sendRequest('someone')).resolves.toEqual({ status: 'sent' });
  });
});

interface Action {
  fn: string;
  args: unknown[];
  binding: string;
}

const ACTIONS: Action[] = [
  { fn: 'sendRequest', args: ['alice'], binding: 'social.SendRequest' },
  { fn: 'accept', args: ['u1'], binding: 'social.Accept' },
  { fn: 'decline', args: ['u1'], binding: 'social.Decline' },
  { fn: 'unfriend', args: ['u1'], binding: 'social.Unfriend' },
  { fn: 'block', args: ['u1'], binding: 'social.Block' },
  { fn: 'unblock', args: ['u1'], binding: 'social.Unblock' },
  { fn: 'friendCode', args: [], binding: 'social.FriendCode' },
  { fn: 'rotateFriendCode', args: [], binding: 'social.RotateFriendCode' },
  { fn: 'react', args: ['e1', 'fire'], binding: 'social.React' },
  { fn: 'unreact', args: ['e1', 'fire'], binding: 'social.Unreact' },
  { fn: 'setNote', args: ['e1', 'nice'], binding: 'social.SetNote' },
  { fn: 'profile', args: ['alice'], binding: 'social.Profile' },
  { fn: 'profileByCode', args: ['TY-1'], binding: 'social.ProfileByCode' },
];

describe('social actions', () => {
  it.each(ACTIONS)('$fn calls $binding with its arguments', async ({ fn, args, binding }) => {
    answer(binding, { id: 'x', status: 'sent' });
    const { social } = await load(true);

    await social[fn](...args);

    expect(calls.filter((call) => call.key === binding)).toEqual([{ key: binding, args }]);
  });

  it.each(ACTIONS)('$fn says the player is signed out when there is no backend', async ({ fn, args }) => {
    const { social } = await load(false);

    const error = await social[fn](...args).catch((err: unknown) => err);

    expect((error as { code: string }).code).toBe('unauthenticated');
    expect(calls).toEqual([]);
  });

  it.each(ACTIONS)('$fn reports a backend refusal as an account error', async ({ fn, args, binding }) => {
    answer(binding, () => {
      throw new Error('rate_limited');
    });
    const { social, AccountError } = await load(true);

    const error = await social[fn](...args).catch((err: unknown) => err);

    expect(error).toBeInstanceOf(AccountError);
    expect((error as { code: string }).code).toBe('rate_limited');
  });
});

describe('public profile', () => {
  it('fills in what an older API did not send', async () => {
    answer('social.Profile', { id: 'u1', username: 'alice', displayName: 'Alice', avatarUrl: '' });
    const { social } = await load(true);

    const profile = (await social.profile('alice')) as unknown as Record<string, unknown>;

    expect(profile).toMatchObject({
      bio: '',
      relation: 'none',
      visibility: '',
      stats: null,
      favorites: [],
      showcase: [],
      recentlyPlayed: [],
      recentActivity: [],
      common: null,
      mutualFriends: [],
      mutualCount: 0,
      createdAt: '',
      presence: null,
    });
  });

  it('keeps what the API did send', async () => {
    answer('social.Profile', {
      id: 'u1',
      username: 'alice',
      displayName: 'Alice',
      avatarUrl: 'a.png',
      bio: 'hi',
      relation: 'friend',
      mutualCount: 4,
      favorites: [{ id: 'g1' }],
    });
    const { social } = await load(true);

    const profile = (await social.profile('alice')) as unknown as Record<string, unknown>;

    expect(profile).toMatchObject({ bio: 'hi', relation: 'friend', mutualCount: 4, favorites: [{ id: 'g1' }] });
  });

  it('reports an empty profile reply as a server error', async () => {
    answer('social.Profile', null);
    const { social } = await load(true);

    const error = await social.profile('ghost').catch((err: unknown) => err);

    expect((error as { code: string }).code).toBe('server_error');
  });
});

describe('lists around a profile', () => {
  it('pages through a user’s games with the cursor, and an empty page has no next cursor', async () => {
    answer('social.UserGames', { games: null, next: undefined });
    const { social } = await load(true);

    await expect(social.userGames('alice', 'c1')).resolves.toEqual({ games: [], next: '' });
    expect(calls.at(-1)).toEqual({ key: 'social.UserGames', args: ['alice', 'c1'] });
  });

  it('is empty without a backend', async () => {
    const { social } = await load(false);

    await expect(social.userGames('alice')).resolves.toEqual({ games: [], next: '' });
    await expect(social.gameFriends('g1')).resolves.toEqual({ played: [], playingNow: [] });
    await expect(social.blocks()).resolves.toEqual([]);
    await expect(social.feed()).resolves.toEqual({ events: [], next: 0 });
    await expect(social.refresh()).resolves.toBeUndefined();
    await expect(social.kick()).resolves.toBeUndefined();
    expect(calls).toEqual([]);
  });

  it('normalises friends of a game and the block list', async () => {
    answer('social.GameFriends', { played: null, playingNow: [{ id: 'u1' }] });
    answer('social.Blocks', null);
    const { social } = await load(true);

    await expect(social.gameFriends('g1')).resolves.toEqual({ played: [], playingNow: [{ id: 'u1' }] });
    await expect(social.blocks()).resolves.toEqual([]);
  });
});

describe('activity feed', () => {
  it('gives every event empty reaction lists and an empty note when Go sent none', async () => {
    answer('social.Feed', { events: [{ id: 'e1', reactions: null, mine: null }, { id: 'e2', reactions: [{ emoji: 'fire', count: 2 }], mine: ['fire'], note: 'gg' }], next: 7 });
    const { social } = await load(true);

    const page = (await social.feed('c0')) as unknown as { events: Array<Record<string, unknown>>; next: number };

    expect(page.next).toBe(7);
    expect(page.events[0]).toMatchObject({ id: 'e1', reactions: [], mine: [], note: '' });
    expect(page.events[1]).toMatchObject({ id: 'e2', reactions: [{ emoji: 'fire', count: 2 }], mine: ['fire'], note: 'gg' });
    expect(calls.at(-1)).toEqual({ key: 'social.Feed', args: ['c0'] });
  });

  it('turns a missing page into an empty one', async () => {
    answer('social.Feed', null);
    const { social } = await load(true);

    await expect(social.feed()).resolves.toEqual({ events: [], next: 0 });
  });
});
