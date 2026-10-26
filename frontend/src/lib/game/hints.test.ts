import { describe, expect, it } from 'vitest';
import { hintKey, shouldSuggestMastered, shouldSuggestPlaying } from './hints';

const none = new Set<string>();

describe('shouldSuggestPlaying', () => {
  it('suggests it for a game that was never played and has no status', () => {
    expect(shouldSuggestPlaying({ status: '', playtimeSeconds: 0 }, 'g1', none)).toBe(true);
    expect(shouldSuggestPlaying({ playtimeSeconds: 0 }, 'g1', none)).toBe(true);
  });

  it('stays quiet when the game already has a status', () => {
    expect(shouldSuggestPlaying({ status: 'backlog', playtimeSeconds: 0 }, 'g1', none)).toBe(false);
    expect(shouldSuggestPlaying({ status: 'dropped', playtimeSeconds: 0 }, 'g1', none)).toBe(false);
  });

  it('stays quiet when the game was played before', () => {
    expect(shouldSuggestPlaying({ status: '', playtimeSeconds: 1 }, 'g1', none)).toBe(false);
  });

  it('stays quiet for a game that is not in the library', () => {
    expect(shouldSuggestPlaying(undefined, 'g1', none)).toBe(false);
  });

  it('does not repeat itself for the same game, but does for another one', () => {
    const shown = new Set([hintKey('playing', 'g1')]);
    expect(shouldSuggestPlaying({ status: '', playtimeSeconds: 0 }, 'g1', shown)).toBe(false);
    expect(shouldSuggestPlaying({ status: '', playtimeSeconds: 0 }, 'g2', shown)).toBe(true);
  });
});

describe('shouldSuggestMastered', () => {
  const full = { total: 12, unlocked: 12 };

  it('suggests it when every achievement is unlocked', () => {
    expect(shouldSuggestMastered({ status: 'completed', playtimeSeconds: 100 }, full, 'g1', none)).toBe(true);
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, full, 'g1', none)).toBe(true);
  });

  it('stays quiet while some achievements are locked', () => {
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, { total: 12, unlocked: 11 }, 'g1', none)).toBe(false);
  });

  it('stays quiet for a game without achievements', () => {
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, { total: 0, unlocked: 0 }, 'g1', none)).toBe(false);
  });

  it('stays quiet when the game is already mastered', () => {
    expect(shouldSuggestMastered({ status: 'mastered', playtimeSeconds: 100 }, full, 'g1', none)).toBe(false);
  });

  it('stays quiet for a game that is not in the library', () => {
    expect(shouldSuggestMastered(undefined, full, 'g1', none)).toBe(false);
  });

  it('does not repeat itself for the same game, and keeps the two hints apart', () => {
    const shown = new Set([hintKey('mastered', 'g1')]);
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, full, 'g1', shown)).toBe(false);
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, full, 'g2', shown)).toBe(true);
    const playing = new Set([hintKey('playing', 'g1')]);
    expect(shouldSuggestMastered({ status: '', playtimeSeconds: 100 }, full, 'g1', playing)).toBe(true);
  });
});
