import { describe, expect, it } from 'vitest';
import { moveErrorText } from './moveMessages';

describe('moveErrorText', () => {
  it.each([
    ['game-id-123: игра сейчас запущена', 'Игра запущена'],
    ['game-id-123: для игры выполняется обновление', 'выполняется обновление'],
    ['game-id-123: для игры выполняется установка', 'идёт установка'],
    ['game-id-123: для игры идёт загрузка', 'идёт загрузка'],
    ['game-id-123: перенос уже выполняется', 'уже выполняется'],
    ['D:\\Games\\Foo: целевой каталог не пуст или недоступен', 'не пуста'],
    ['целевой каталог не может быть внутри исходного: D:\\Games', 'внутри исходной'],
    ['исходный каталог не может быть внутри целевого: C:\\Games', 'внутри целевой'],
    ['целевой каталог не может быть корнем диска: D:\\', 'корень диска'],
    ['не удалось определить свободное место на диске: stat D:\\: access denied', 'свободное место'],
    ['недостаточно свободного места на диске: нужно 500 байт, доступно 100', 'Недостаточно места'],
    ['проверка перенесённых файлов не прошла: checksum mismatch', 'Проверка перенесённых файлов не прошла'],
    ['game-id-123: у игры не задан каталог установки', 'не задан каталог установки'],
  ])('translates %s', (raw, expected) => {
    expect(moveErrorText(new Error(raw))).toContain(expected);
  });

  it('falls back for an unknown error', () => {
    expect(moveErrorText(new Error('something unexpected'), 'Не удалось перенести')).toBe('Не удалось перенести');
  });

  it('falls back for an empty message', () => {
    expect(moveErrorText(new Error(''), 'Резервный текст')).toBe('Резервный текст');
  });

  it('accepts a raw string message', () => {
    expect(moveErrorText('перенос уже выполняется')).toBe('Перенос уже выполняется');
  });

  it('falls back when there is nothing to read', () => {
    expect(moveErrorText(undefined, 'Резервный текст')).toBe('Резервный текст');
  });
});
