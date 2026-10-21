import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { playerCardOf } from './appearanceCard';
import { DARK_INK } from './appearanceColor';

const base = { accent: '#ff8800', avatarFrame: 'ring' as const, nameStyle: 'glow' as const };

describe('playerCardOf colours', () => {
  it('carries the name and frame colours into the appearance', () => {
    const view = playerCardOf({ ...base, nameColor: '#112233', frameColor: '#AABBCC' });
    expect(view.appearance.nameColor).toBe('#112233');
    expect(view.appearance.frameColor).toBe('#AABBCC');
  });
  it('leaves them empty, meaning the accent, when absent or broken', () => {
    for (const card of [undefined, null, base, { ...base, nameColor: 'red', frameColor: '#12' }]) {
      const view = playerCardOf(card);
      expect(view.appearance.nameColor).toBe('');
      expect(view.appearance.frameColor).toBe('');
    }
  });
  it('gives the card dark ink for a light gradient shown as is', () => {
    const card = { ...base, theme: 'custom', customFrom: '#ffffff', customTo: '#f3e6c8' };
    const pure = playerCardOf({ ...card, customPure: true });
    expect(pure.theme.ink).toBe(DARK_INK);
    expect(pure.theme.background).not.toBe(playerCardOf(card).theme.background);
    expect(playerCardOf(card).theme.ink).toBeUndefined();
  });
});
