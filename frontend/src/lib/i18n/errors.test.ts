import { describe, expect, it } from 'vitest';
import { errorCode, hasMessage } from './errors';

describe('errorCode', () => {
  it('reads the code the backend puts in the message', () => {
    expect(errorCode(new Error('typhon:relocate.game_running: игра сейчас запущена'))).toBe(
      'relocate.game_running',
    );
  });

  it('finds the code inside a wrapped message', () => {
    expect(
      errorCode(new Error('game-id-123: typhon:relocate.not_enough_space: нужно 500 байт')),
    ).toBe('relocate.not_enough_space');
  });

  it('accepts a plain string', () => {
    expect(errorCode('typhon:install.disk_full: no space')).toBe('install.disk_full');
  });

  it('returns nothing for an error without a code', () => {
    expect(errorCode(new Error('something unexpected'))).toBe('');
    expect(errorCode(new Error(''))).toBe('');
    expect(errorCode(undefined)).toBe('');
    expect(errorCode(null)).toBe('');
  });

  it('does not treat the prefix alone as a code', () => {
    expect(errorCode(new Error('typhon: что-то'))).toBe('');
  });
});

describe('hasMessage', () => {
  it('recognises a key the catalog defines', () => {
    expect(hasMessage('common.cancel')).toBe(true);
  });

  it('rejects a key the catalog does not define', () => {
    expect(hasMessage('nope.missing')).toBe(false);
  });
});
