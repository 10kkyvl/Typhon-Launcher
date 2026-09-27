import { describe, expect, it } from 'vitest';
import { NOTE_LIMIT, NOTE_PREVIEW, noteLength, notePreview, trimNote } from './note';

const long = (count: number, char = 'я') => char.repeat(count);

describe('noteLength', () => {
  it('считает символы, а не байты', () => {
    expect(noteLength('привет')).toBe(6);
    expect(noteLength('')).toBe(0);
  });

  it('считает эмодзи из суррогатной пары за один символ', () => {
    expect(noteLength('🔥')).toBe(1);
    expect(noteLength('гвинт 🔥')).toBe(7);
  });
});

describe('trimNote', () => {
  it('срезает пробелы по краям', () => {
    expect(trimNote('  прошёл спустя три года  ')).toBe('прошёл спустя три года');
  });

  it('схлопывает строку из одних пробелов в пустую', () => {
    expect(trimNote('   \n  ')).toBe('');
  });
});

describe('notePreview', () => {
  it('короткую подпись отдаёт целиком', () => {
    const note = 'Прошёл спустя три года.';
    expect(notePreview(note)).toEqual({ text: note, truncated: false });
  });

  it('подпись ровно в лимит превью не режет', () => {
    const note = long(NOTE_PREVIEW);
    expect(notePreview(note)).toEqual({ text: note, truncated: false });
  });

  it('длинную подпись режет и помечает как обрезанную', () => {
    const preview = notePreview(long(NOTE_PREVIEW + 50));
    expect(preview.truncated).toBe(true);
    expect(noteLength(preview.text)).toBeLessThanOrEqual(NOTE_PREVIEW);
  });

  it('режет по границе слова, а не посреди него', () => {
    const note = `${long(NOTE_PREVIEW - 6)} Ведьмак и Цири`;
    const preview = notePreview(note);
    expect(preview.truncated).toBe(true);
    expect(preview.text.endsWith('Ведь')).toBe(false);
    expect(preview.text).toBe(long(NOTE_PREVIEW - 6));
  });

  it('режет по лимиту, когда слово длиннее превью', () => {
    const preview = notePreview(long(NOTE_PREVIEW + 100));
    expect(noteLength(preview.text)).toBe(NOTE_PREVIEW);
  });

  it('не считает эмодзи за два символа при обрезке', () => {
    const preview = notePreview('🔥'.repeat(NOTE_PREVIEW + 10));
    expect(noteLength(preview.text)).toBe(NOTE_PREVIEW);
    expect(preview.text.endsWith('🔥')).toBe(true);
  });

  it('пустую подпись отдаёт пустой', () => {
    expect(notePreview('')).toEqual({ text: '', truncated: false });
  });

  it('превью короче лимита ввода', () => {
    expect(NOTE_PREVIEW).toBeLessThan(NOTE_LIMIT);
  });
});
