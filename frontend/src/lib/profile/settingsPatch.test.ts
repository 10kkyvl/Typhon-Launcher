import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import type { ProfileSettings } from '../services/account';
import { DEFAULT_PROFILE } from '../services/account';
import { DEFAULT_APPEARANCE } from './appearance';
import { privacyPatch } from './settingsPatch';

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
