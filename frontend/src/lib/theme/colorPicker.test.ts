import { expect, it } from 'vitest';
import { hexToHsv, hsvToHex, clampUnit } from './colorPicker';

it('round trips selected colors, including grayscale and extreme channels', () => {
  for (const color of ['#000000', '#FFFFFF', '#FFFF00', '#4287F5', '#16A6B6', '#808080', '#FF0000', '#00FF00', '#0000FF']) {
    expect(hsvToHex(hexToHsv(color))).toBe(color);
  }
});
it('retains hue for grayscale and maps the full hue strip without a seam', () => {
  expect(hexToHsv('#000000', 217).h).toBe(217);
  expect(hexToHsv('#FFFFFF', 217).h).toBe(217);
  expect(hsvToHex({ h: 0, s: 1, v: 1 })).toBe('#FF0000');
  expect(hsvToHex({ h: 360, s: 1, v: 1 })).toBe('#FF0000');
});
it('clamps pointer coordinates outside the color field', () => {
  expect(clampUnit(-.3)).toBe(0);
  expect(clampUnit(1.4)).toBe(1);
  expect(hsvToHex({ h: 120, s: 2, v: 2 })).toBe('#00FF00');
});
