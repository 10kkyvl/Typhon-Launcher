import { describe, expect, it } from 'vitest';
import { rateLimitLabel, rateMbText } from './format';

const MB = 1024 * 1024;

describe('rateLimitLabel', () => {
  it.each([
    [0, 'Без ограничений'],
    [-1, 'Без ограничений'],
    [10 * MB, '10 МБ/с'],
    [Math.round(37.5 * MB), '37,5 МБ/с'],
    [Math.round(0.1 * MB), '0,1 МБ/с'],
    [Math.round(1000 * MB), '1000 МБ/с'],
  ])('formats %i bytes/s as %s', (bytes, expected) => {
    expect(rateLimitLabel(bytes)).toBe(expected);
  });
});

describe('rateMbText', () => {
  it.each([
    [Math.round(2.25 * MB), '2,25'],
    [Math.round(2.256 * MB), '2,26'],
    [3 * MB, '3'],
  ])('renders %i bytes/s as %s', (bytes, expected) => {
    expect(rateMbText(bytes)).toBe(expected);
  });
});
