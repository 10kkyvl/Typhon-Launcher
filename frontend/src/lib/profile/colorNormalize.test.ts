import { afterEach, describe, expect, it, vi } from 'vitest';
import { normalizeColor } from './colorNormalize';

afterEach(() => vi.unstubAllGlobals());

describe('normalizeColor', () => {
  it('turns every common spelling into lowercase #rrggbb', () => {
    expect(normalizeColor('#ABC')).toBe('#aabbcc');
    expect(normalizeColor('#4F6BFF')).toBe('#4f6bff');
    expect(normalizeColor('rgb(79, 107, 255)')).toBe('#4f6bff');
    expect(normalizeColor('rgb(79 107 255 / 100%)')).toBe('#4f6bff');
    expect(normalizeColor('rgb(100% 0% 0%)')).toBe('#ff0000');
    expect(normalizeColor('hsl(0, 100%, 50%)')).toBe('#ff0000');
    expect(normalizeColor('hsl(120deg 100% 25%)')).toBe('#008000');
    expect(normalizeColor('  #fff  ')).toBe('#ffffff');
  });

  it('refuses what is not an opaque colour', () => {
    for (const bad of ['', 'not a colour', 'var(--accent)', 'rgba(1, 2, 3, 0.5)', 'rgb(1, 2)', 'hsl(0, 100, 50)', '#12345']) {
      expect(normalizeColor(bad), bad).toBeNull();
    }
  });

  it('hands other forms to the browser and rejects what it does not understand', () => {
    const ctx = { _value: '#000000', get fillStyle() { return this._value; }, set fillStyle(v: string) { if (v === 'oklch(0.7 0.1 200)') this._value = '#33aabb'; else if (/^#/.test(v)) this._value = v; } };
    vi.stubGlobal('document', { createElement: () => ({ getContext: () => ctx }) });
    expect(normalizeColor('oklch(0.7 0.1 200)')).toBe('#33aabb');
    expect(normalizeColor('banana')).toBeNull();
  });

  it('gives up without a document', () => {
    expect(normalizeColor('oklch(0.7 0.1 200)')).toBeNull();
  });
});
