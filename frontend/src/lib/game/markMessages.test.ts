import { describe, expect, it } from 'vitest';
import { msg } from '../i18n';
import { markError } from './markMessages';

describe('markError', () => {
  it('says the favourites limit for its code', () => {
    expect(markError(new Error('typhon:library.too_many_favorites: favorites limit reached'), 'запасной текст')).toBe('Не больше 6 любимых игр');
  });

  it('reads the code, not the prose: a renamed message changes nothing', () => {
    expect(markError(new Error('typhon:library.too_many_favorites: достигнут лимит избранного'), 'запасной текст')).toBe(msg('games.markFavoritesLimit'));
    expect(markError(new Error('favorites limit reached'), 'запасной текст')).toBe('запасной текст');
  });

  it('shows the text of any other code it has words for', () => {
    expect(markError(new Error('typhon:library.invalid_status: invalid game status'), 'запасной текст')).toBe(msg('library.invalid_status'));
  });

  it('falls back for a code without text', () => {
    expect(markError(new Error('typhon:library.no_such_code_here: boom'), 'запасной текст')).toBe('запасной текст');
  });

  it('falls back for an unrecognized error', () => {
    expect(markError(new Error('something else'), 'запасной текст')).toBe('запасной текст');
  });

  it('falls back for a non-Error value', () => {
    expect(markError('boom', 'запасной текст')).toBe('запасной текст');
  });
});
