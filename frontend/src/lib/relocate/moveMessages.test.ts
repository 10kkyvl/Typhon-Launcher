import { describe, expect, it } from 'vitest';
import { moveErrorText } from './moveMessages';

describe('moveErrorText', () => {
  it.each([
    ['game-id-123: typhon:relocate.game_running: игра сейчас запущена', 'Игра запущена'],
    ['game-id-123: typhon:relocate.updating: для игры выполняется обновление', 'выполняется обновление'],
    ['game-id-123: typhon:relocate.installing: для игры выполняется установка', 'идёт установка'],
    ['game-id-123: typhon:relocate.downloading: для игры идёт загрузка', 'идёт загрузка'],
    ['game-id-123: typhon:relocate.already_running: перенос уже выполняется', 'уже выполняется'],
    ['D:\\Games\\Foo: typhon:relocate.target_not_empty: целевой каталог не пуст', 'не пуста'],
    ['typhon:relocate.target_inside_source: целевой каталог внутри исходного', 'внутри исходной'],
    ['typhon:relocate.source_inside_target: исходный каталог внутри целевого', 'внутри целевой'],
    ['typhon:relocate.target_is_drive_root: корень диска D:\\', 'корень диска'],
    ['typhon:relocate.free_space_unknown: stat D:\\: access denied', 'свободное место'],
    ['typhon:relocate.not_enough_space: нужно 500 байт, доступно 100', 'Недостаточно места'],
    ['typhon:relocate.verify_failed: checksum mismatch', 'Проверка перенесённых файлов не прошла'],
    ['game-id-123: typhon:relocate.no_install_dir: у игры не задан каталог установки', 'не задан каталог установки'],
  ])('translates %s', (raw, expected) => {
    expect(moveErrorText(new Error(raw))).toContain(expected);
  });

  it('falls back when the backend text carries no code', () => {
    expect(moveErrorText(new Error('игра сейчас запущена'), 'Не удалось перенести')).toBe(
      'Не удалось перенести',
    );
  });

  it('falls back for an unknown error', () => {
    expect(moveErrorText(new Error('something unexpected'), 'Не удалось перенести')).toBe('Не удалось перенести');
  });

  it('falls back for an empty message', () => {
    expect(moveErrorText(new Error(''), 'Резервный текст')).toBe('Резервный текст');
  });

  it('accepts a raw string message', () => {
    expect(moveErrorText('typhon:relocate.already_running: перенос уже выполняется')).toBe(
      'Перенос уже выполняется',
    );
  });

  it('falls back when there is nothing to read', () => {
    expect(moveErrorText(undefined, 'Резервный текст')).toBe('Резервный текст');
  });
});
