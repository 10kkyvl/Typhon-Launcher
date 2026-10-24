import { describe, expect, it } from 'vitest';
import { tokenGroups } from '../theme/tokens';
import { bigPicturePalette, bigPictureStyle } from './palette';
import { goSources } from '../testing/sources';

const neutral = tokenGroups
  .filter((group) => group.id === 'background' || group.id === 'surfaces' || group.id === 'text')
  .flatMap((group) => [...group.tokens]);

describe('bigPicturePalette', () => {
  it('covers every neutral theme token', () => {
    expect(Object.keys(bigPicturePalette).sort()).toEqual([...neutral].sort());
  });

  it('never overrides the accent or status tokens', () => {
    for (const name of Object.keys(bigPicturePalette)) {
      expect(name).not.toMatch(/^--(accent|success|warning|danger)/);
    }
  });

  it('takes its values from the dark preset', () => {
    const source = goSources['internal/theme/presets.go'];
    expect(source).toBeTypeOf('string');
    const dark = source.slice(source.indexOf('var presetDark'));
    const block = dark.slice(0, dark.search(/^}/m));
    const preset = new Map([...block.matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((match) => [match[1], match[2]]));
    for (const [name, value] of Object.entries(bigPicturePalette)) {
      expect(preset.get(name), name).toBe(value);
    }
  });

  it('serialises every token into the inline style', () => {
    for (const [name, value] of Object.entries(bigPicturePalette)) {
      expect(bigPictureStyle).toContain(`${name}: ${value}`);
    }
  });
});
