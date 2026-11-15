import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('../services/backend', () => ({ inWails: false }));

// The art the backend has: what GetArt answers (games without art are left out) and what EnsureArt takes.
const backend = vi.hoisted(() => ({
  getGameArt: vi.fn(async (_ids: string[]): Promise<Record<string, unknown>> => ({})),
  ensureArt: vi.fn(async (ids: string[]) => ids),
}));
vi.mock('../services/metadata', async (original) => ({
  ...(await original<typeof import('../services/metadata')>()),
  getGameArt: (ids: string[]) => backend.getGameArt(ids),
  ensureArt: (ids: string[]) => backend.ensureArt(ids),
}));

type Stage = typeof import('./stage');
type Library = typeof import('./library');
type Metadata = typeof import('./metadata');
let stage: Stage;
let library: Library;
let metadata: Metadata;

beforeEach(async () => {
  backend.getGameArt.mockReset();
  backend.getGameArt.mockImplementation(async () => ({}));
  backend.ensureArt.mockReset();
  backend.ensureArt.mockImplementation(async (ids: string[]) => ids);
  vi.resetModules();
  library = await import('./library');
  metadata = await import('./metadata');
  stage = await import('./stage');
});

// A release is published after the current task: what the stage shows is read once it has run.
const settled = () => Promise.resolve();

describe('which game stands on the stage', () => {
  it('none before any screen claims one', () => {
    expect(get(stage.stageGame)).toBeNull();
  });

  it('the newest live claim wins and an older one takes over when it is released', async () => {
    const first = stage.claimStage('a');
    const second = stage.claimStage('b');
    expect(get(stage.stageGame)).toBe('b');
    second();
    await settled();
    expect(get(stage.stageGame)).toBe('a');
    first();
    await settled();
    expect(get(stage.stageGame)).toBeNull();
  });

  it('has no memory: once the last claim is released the stage is empty, not the last game', async () => {
    stage.claimStage('elden')();
    await settled();
    expect(get(stage.stageGame)).toBeNull();
  });

  it('releases in any order leave no claim behind', async () => {
    const first = stage.claimStage('a');
    const second = stage.claimStage('b');
    first();
    await settled();
    expect(get(stage.stageGame)).toBe('b');
    second();
    await settled();
    expect(get(stage.stageGame)).toBeNull();
  });

  it('a release runs once: calling it again does not remove another claim of the same game', async () => {
    const first = stage.claimStage('a');
    const second = stage.claimStage('a');
    first();
    first();
    await settled();
    expect(get(stage.stageGame)).toBe('a');
    second();
    await settled();
    expect(get(stage.stageGame)).toBeNull();
  });

  it('a claim of no game claims nothing and never wipes another claimant', async () => {
    const release = stage.claimStage('a');
    const none = stage.claimStage(null);
    expect(get(stage.stageGame)).toBe('a');
    none();
    await settled();
    expect(get(stage.stageGame)).toBe('a');
    release();
  });

  it('one screen giving way to the next never shows the window without a game in between', async () => {
    const seen: (string | null)[] = [];
    const stop = stage.stageGame.subscribe((id) => seen.push(id));
    const old = stage.claimStage('a');
    old();
    const next = stage.claimStage('b');
    await settled();
    expect(seen).toEqual([null, 'a', 'b']);
    next();
    stop();
  });
});

describe('whether a game has wide art for the stage', () => {
  const answer = (id: string) => get(stage.stageWide(id));

  it('is not known yet until the art store answers, then yes with the picture', () => {
    library.libraryGames.set([{ id: 'lib-1', title: 'One', executable: '', installDir: '', cover: '/media/own.jpg', version: '', sizeBytes: 0, lastPlayed: null, playtimeSeconds: 0, installedAt: '', canonicalGameId: 'c1' }]);
    expect(answer('lib-1')).toEqual({ state: 'unknown' });
    metadata.gameArt.set({ c1: { cover: '/media/cover.jpg', hero: '/media/hero.jpg', heroIsPoster: false } });
    expect(answer('lib-1')).toEqual({ state: 'yes', src: '/media/hero.jpg' });
  });

  it('is no once the backend has answered without art for the game: it never stays unknown', async () => {
    backend.getGameArt.mockImplementation(async () => ({}));
    expect(answer('c9')).toEqual({ state: 'unknown' });
    await metadata.loadArt(['c9']);
    expect(backend.getGameArt).toHaveBeenCalledWith(['c9']);
    expect(answer('c9')).toEqual({ state: 'no' });
  });

  it('is no once the stage\'s own request for art comes back without any', async () => {
    backend.getGameArt.mockImplementation(async () => ({}));
    metadata.requestArt(['c10']);
    await vi.waitFor(() => expect(answer('c10')).toEqual({ state: 'no' }));
  });

  it('turns to yes when art arrives for a game that was answered without any', async () => {
    await metadata.loadArt(['c11']);
    expect(answer('c11')).toEqual({ state: 'no' });
    backend.getGameArt.mockImplementation(async () => ({}));
    metadata.gameArt.update((map) => ({ ...map, c11: { cover: '', hero: '/media/late.jpg', heroIsPoster: false } }));
    expect(answer('c11')).toEqual({ state: 'yes', src: '/media/late.jpg' });
  });

  it('is no when asking the backend failed: nothing would come for the game until it is asked again', async () => {
    backend.getGameArt.mockImplementation(async () => {
      throw new Error('typhon:metadata.unavailable');
    });
    await metadata.loadArt(['c12']);
    expect(answer('c12')).toEqual({ state: 'no' });
  });

  it('a game with art is yes from the first answer, never no on the way', async () => {
    backend.getGameArt.mockImplementation(async () => ({ c13: { cover: '', hero: '/media/h13.jpg', heroIsPoster: false } }));
    const seen: string[] = [];
    const stop = stage.stageWide('c13').subscribe((wide) => seen.push(wide.state));
    await metadata.loadArt(['c13']);
    expect(seen[0]).toBe('unknown');
    expect(seen.slice(1).every((state) => state === 'yes')).toBe(true);
    expect(seen.length).toBeGreaterThan(1);
    stop();
  });

  it('is no for a game whose hero is its poster', () => {
    metadata.gameArt.set({ c2: { cover: '/media/cover.jpg', hero: '/media/cover.jpg', heroIsPoster: true } });
    expect(answer('c2')).toEqual({ state: 'no' });
  });

  it('turns to no once the stage could not load the picture, and stays yes for another picture', () => {
    metadata.gameArt.set({
      c3: { cover: '', hero: '/media/broken.jpg', heroIsPoster: false },
      c4: { cover: '', hero: '/media/fine.jpg', heroIsPoster: false },
    });
    expect(answer('c3')).toEqual({ state: 'yes', src: '/media/broken.jpg' });
    stage.stagePictureFailed('/media/broken.jpg');
    expect(answer('c3')).toEqual({ state: 'no' });
    expect(answer('c4')).toEqual({ state: 'yes', src: '/media/fine.jpg' });
  });

  it('follows the art store while the claimer listens', () => {
    const seen: string[] = [];
    const stop = stage.stageWide('c5').subscribe((wide) => seen.push(wide.state));
    metadata.gameArt.set({ c5: { cover: '', hero: '/media/h.jpg', heroIsPoster: false } });
    expect(seen).toEqual(['unknown', 'yes']);
    stop();
  });
});

describe('the picture the stage draws', () => {
  it('is the staged game and its wide art, or nothing without a claim', async () => {
    metadata.gameArt.set({ c1: { cover: '', hero: '/media/hero.jpg', heroIsPoster: false } });
    expect(get(stage.stagePicture)).toBeNull();
    const release = stage.claimStage('c1');
    expect(get(stage.stagePicture)).toEqual({ id: 'c1', src: '/media/hero.jpg' });
    release();
    await settled();
    expect(get(stage.stagePicture)).toBeNull();
  });

  it('keeps the claim with an empty picture for a game without wide art', () => {
    metadata.gameArt.set({ c2: { cover: '/media/cover.jpg', hero: '/media/cover.jpg', heroIsPoster: true } });
    const release = stage.claimStage('c2');
    expect(get(stage.stagePicture)).toEqual({ id: 'c2', src: '' });
    release();
  });
});
