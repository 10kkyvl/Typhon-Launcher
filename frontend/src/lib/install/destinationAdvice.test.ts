import { describe, expect, it } from 'vitest';
import { adviseDestination } from './destinationAdvice';

const free = { destinationBusy: false, suggested: 'E:\\Games\\Doom', reusable: false };
const busy = { destinationBusy: true, suggested: 'E:\\Games\\Doom', reusable: false };

describe('adviseDestination', () => {
  it('switches a picked busy folder to the suggestion', () => {
    expect(adviseDestination(busy, 'pick', 'E:\\Games')).toEqual({
      destination: 'E:\\Games\\Doom',
      hint: { kind: 'adjusted', path: 'E:\\Games\\Doom' },
    });
  });

  it('keeps typed text and offers the suggestion', () => {
    expect(adviseDestination(busy, 'typed', 'E:\\Games')).toEqual({
      destination: 'E:\\Games',
      hint: { kind: 'suggest', path: 'E:\\Games\\Doom' },
    });
  });

  it('says nothing when the folder is busy and no better name came back', () => {
    expect(adviseDestination({ ...busy, suggested: '' }, 'pick', 'E:\\Games')).toEqual({
      destination: 'E:\\Games',
      hint: null,
    });
  });

  it('says nothing when the suggestion is the folder itself', () => {
    expect(adviseDestination(busy, 'pick', 'e:/games/doom/')).toEqual({
      destination: 'e:/games/doom/',
      hint: null,
    });
  });

  it('warns about leftovers of a failed install without touching the field', () => {
    expect(adviseDestination({ ...free, reusable: true }, 'pick', 'E:\\Games\\Doom')).toEqual({
      destination: 'E:\\Games\\Doom',
      hint: { kind: 'reusable', path: 'E:\\Games\\Doom' },
    });
  });

  it('does nothing for a free folder', () => {
    expect(adviseDestination(free, 'pick', 'E:\\Games\\Doom')).toEqual({
      destination: 'E:\\Games\\Doom',
      hint: null,
    });
    expect(adviseDestination(free, 'typed', 'E:\\Other')).toEqual({ destination: 'E:\\Other', hint: null });
  });
});
