import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import type { ProfileSettings } from '../services/account';
import { DEFAULT_PROFILE } from '../services/account';
import { DEFAULT_APPEARANCE } from './appearance';
import { privacyPatch, withFeedKind } from './settingsPatch';

const stale: ProfileSettings = {
  ...DEFAULT_PROFILE,
  showcase: ['favorites'],
  statusEmoji: '🎮',
  statusText: 'old status',
  layout: { version: 1, blocks: [{ id: 'a1', type: 'about', width: 'full', config: {} }] },
};

describe('privacyPatch', () => {
  it('never carries the layout or the status, so the server keeps what another device wrote', () => {
    const patch = privacyPatch(stale, 'public');

    expect(patch).not.toHaveProperty('layout');
    expect(patch).not.toHaveProperty('statusEmoji');
    expect(patch).not.toHaveProperty('statusText');
  });

  it('never carries the appearance, so the server keeps what another device chose', () => {
    const patch = privacyPatch({ ...stale, appearance: { ...DEFAULT_APPEARANCE, theme: 'forest' } }, 'public');

    expect(Object.keys(patch)).not.toContain('appearance');
  });

  it('carries the visibility, every flag the modal edits, and nothing else', () => {
    const patch = privacyPatch({ ...stale, showPlaytime: false, showStats: false }, 'private');

    expect(Object.keys(patch).sort()).toEqual(
      ['showActivity', 'showLibrary', 'showOnline', 'showPlaying', 'showPlaytime', 'showStats', 'visibility'].sort(),
    );
    expect(patch).toMatchObject({ visibility: 'private', showPlaytime: false, showStats: false, showOnline: true });
  });

  it('never carries the showcase, so the server keeps the one another device stored', () => {
    const patch = privacyPatch(stale, 'friends');

    expect(patch).not.toHaveProperty('showcase');
  });
});

describe('feedHidden', () => {
  it('is not sent when it equals the stored value, whatever the order', () => {
    expect(privacyPatch(stale, 'friends', stale)).not.toHaveProperty('feedHidden');
    expect(privacyPatch({ ...stale, feedHidden: [] }, 'friends', { ...stale, feedHidden: null })).not.toHaveProperty('feedHidden');
    const stored = { ...stale, feedHidden: ['played', 'planned'] };
    expect(privacyPatch({ ...stored, feedHidden: ['planned', 'played'] }, 'friends', stored)).not.toHaveProperty('feedHidden');
  });

  it('is not sent after toggling a kind off and on again', () => {
    const hidden = withFeedKind(withFeedKind(stale.feedHidden, 'played', false), 'played', true);
    expect(privacyPatch({ ...stale, feedHidden: hidden }, 'friends', stale)).not.toHaveProperty('feedHidden');
  });

  it('sends the full list when it differs from the stored one', () => {
    const stored = { ...stale, feedHidden: ['played'] };
    expect(privacyPatch({ ...stored, feedHidden: ['played', 'planned'] }, 'friends', stored)).toMatchObject({
      feedHidden: ['played', 'planned'],
    });
  });

  it('sends an empty list when the user re-enabled everything that was hidden', () => {
    const stored = { ...stale, feedHidden: ['played'] };
    expect(privacyPatch({ ...stored, feedHidden: [] }, 'friends', stored)).toMatchObject({ feedHidden: [] });
  });

  it('round-trips through the toggle helper', () => {
    const hidden = withFeedKind(withFeedKind(undefined, 'played', false), 'planned', false);
    expect(hidden).toEqual(['played', 'planned']);
    expect(withFeedKind(hidden, 'played', true)).toEqual(['planned']);
    expect(privacyPatch({ ...stale, feedHidden: hidden }, 'friends', stale).feedHidden).toEqual(['played', 'planned']);
  });
});
