import { describe, expect, it } from 'vitest';
import { locale, translator } from '../i18n';
import { makeFormat } from '../utils/format';
import { renderDom } from '../testing/dom';
import Reading, { readingParts } from './Reading.svelte';

const MB = 1024 ** 2;
const ru = makeFormat('ru', translator('ru'));
const en = makeFormat('en', translator('en'));
const RU_UNITS = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ', 'КБ/с', 'МБ/с', 'ч', 'мин', 'сек'];

const shape = (text: string, units = RU_UNITS) => readingParts(text, units).map((p) => `${p.kind}:${p.text}`);

describe('a reading cut into number, unit and text', () => {
  it('sets the unit of a speed, a size and a share apart from its number', () => {
    expect(shape(ru.speedBytes(12.5 * MB))).toEqual(['num:12,5', 'text: ', 'unit:МБ/с']);
    expect(shape(ru.bytesSize(3.2 * 1024 * MB))).toEqual(['num:3,2', 'text: ', 'unit:ГБ']);
    expect(shape('54%')).toEqual(['num:54', 'unit:%']);
  });

  it('keeps a number with group separators whole', () => {
    expect(shape('12 345 МБ')).toEqual(['num:12 345', 'text: ', 'unit:МБ']);
  });

  it('reads a time left with two numbers and two units', () => {
    expect(shape(ru.etaLabel(4320))).toEqual(['num:1', 'text: ', 'unit:ч', 'text: ', 'num:12', 'text: ', 'unit:мин']);
  });

  it('marks only words the catalogue knows as units, and leaves the rest as text', () => {
    expect(shape('3 из 12 файлов')).toEqual(['num:3', 'text: из ', 'num:12', 'text: файлов']);
    expect(shape('пик 41,7 МБ/с')).toEqual(['text:пик ', 'num:41,7', 'text: ', 'unit:МБ/с']);
    expect(shape('5 минут')).toEqual(['num:5', 'text: минут']);
  });

  it('a reading with no number is text', () => {
    expect(shape('Без ограничений')).toEqual(['text:Без ограничений']);
    expect(shape('—')).toEqual(['text:—']);
    expect(shape('')).toEqual([]);
  });

  it('a minus on an axis mark is part of its number', () => {
    expect(shape('−30 сек')).toEqual(['num:−30', 'text: ', 'unit:сек']);
  });
});

describe('Reading in a DOM', () => {
  it('says the same words it was given, with the number in tabular figures and the unit set apart', () => {
    locale.set('ru');
    const text = ru.speedBytes(12.5 * MB);
    const { container } = renderDom(Reading, { text });
    const reading = container.querySelector('.reading') as HTMLElement;

    expect(reading.textContent).toBe(text);
    expect(reading.querySelector('.num')?.textContent).toBe('12,5');
    expect(reading.querySelector('.unit')?.textContent).toBe('МБ/с');
  });

  it('takes the units of the language shown', () => {
    locale.set('en');
    const text = en.speedBytes(12.5 * MB);
    const { container } = renderDom(Reading, { text });

    expect(container.querySelector('.reading')?.textContent).toBe('12.5 MB/s');
    expect(container.querySelector('.unit')?.textContent).toBe('MB/s');
  });
});
