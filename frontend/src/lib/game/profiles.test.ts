import { describe, expect, it } from 'vitest';
import { lastProfileId, playLabel, playProfileItems, profileChoices, profileIdFromItem, withProfileItems } from './profiles';

const extra = [
  { id: 'a1', name: 'С модами', executable: 'mod.exe' },
  { id: 'b2', name: 'Оффлайн', executable: 'off.exe' },
];

describe('profiles', () => {
  it('has no choices without extra profiles', () => {
    expect(profileChoices({})).toEqual([]);
    expect(profileChoices({ profiles: [] })).toEqual([]);
    expect(playProfileItems({ profiles: null })).toEqual([]);
  });

  it('lists the default profile first', () => {
    const ids = profileChoices({ profiles: extra }).map((p) => p.id);
    expect(ids).toEqual(['', 'a1', 'b2']);
  });

  it('falls back to the default when the last profile is gone', () => {
    expect(lastProfileId({ profiles: extra, lastProfile: 'b2' })).toBe('b2');
    expect(lastProfileId({ profiles: extra, lastProfile: 'zz' })).toBe('');
    expect(lastProfileId({ profiles: [], lastProfile: 'b2' })).toBe('');
  });

  it('names the profile in the play label only when it is not the default', () => {
    expect(playLabel({ profiles: extra, lastProfile: 'a1' })).toContain('С модами');
    expect(playLabel({ profiles: extra, lastProfile: '' })).not.toContain('С модами');
    expect(playLabel({ profiles: extra, lastProfile: 'zz' })).not.toContain('zz');
  });

  it('round-trips the profile id through a menu item id', () => {
    const items = playProfileItems({ profiles: extra });
    expect(items.map((i) => profileIdFromItem(i.id))).toEqual(['', 'a1', 'b2']);
    expect(profileIdFromItem('properties')).toBeNull();
  });
});

describe('withProfileItems', () => {
  const base = [{ id: 'play', label: 'p' }, { id: 'tags', label: 't' }];

  it('inserts the profile entries right after play', () => {
    const out = withProfileItems(base, { profiles: extra }).map((i) => i.id);
    expect(out).toEqual(['play', 'play-profile:', 'play-profile:a1', 'play-profile:b2', 'tags']);
  });

  it('leaves the menu alone without extra profiles or without play', () => {
    expect(withProfileItems(base, {})).toBe(base);
    const running = [{ id: 'stop', label: 's' }];
    expect(withProfileItems(running, { profiles: extra })).toBe(running);
  });
});
