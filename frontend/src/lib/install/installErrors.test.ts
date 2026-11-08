import { describe, expect, it } from 'vitest';
import { canRetryInstall, installErrorText, needsInteractiveInstall } from './installErrors';

describe('installer refusals', () => {
  it('asks for a manual install when the wizard refused silent mode', () => {
    const err = 'typhon:install.installer_needs_interactive: установщик не поддерживает тихую установку: код 1 (установщик не смог запуститься)';
    expect(needsInteractiveInstall(err)).toBe(true);
    expect(installErrorText(err)).toBe('установщик не поддерживает тихую установку: установите игру вручную');
  });

  it('keeps an ordinary failure on retry', () => {
    expect(needsInteractiveInstall('typhon:install.installer_failed: установщик завершился с ошибкой: код 4')).toBe(false);
    expect(needsInteractiveInstall('')).toBe(false);
    expect(needsInteractiveInstall(undefined)).toBe(false);
  });

  it('maps both seeding stop failures to the same text', () => {
    const expected = 'не удалось остановить раздачу перед удалением';
    expect(installErrorText('typhon:install.remove_seeding: не удалось остановить раздачу: x')).toBe(expected);
    expect(installErrorText('typhon:download.seeding: не удалось остановить раздачу: x')).toBe(expected);
  });

  it('names an interrupted install instead of the fallback', () => {
    expect(installErrorText('typhon:install.interrupted: установка была прервана')).toBe('установка была прервана');
  });

  it('names an unreadable install folder instead of the fallback', () => {
    expect(installErrorText('typhon:install.dest_unreadable: не удалось проверить папку установки: E:\\Games: access denied')).toBe(
      'не удалось проверить папку установки',
    );
  });

  it('carries the suggested subfolder from a busy install folder into the text', () => {
    const err = 'typhon:install.dest_not_empty: папка установки уже существует и не пуста: выберите подпапку, например E:\\Games\\Doom 2';
    expect(installErrorText(err)).toBe(
      'папка установки уже существует и не пуста, выберите подпапку, например E:\\Games\\Doom 2',
    );
  });

  it('falls back to the fixed text when the busy folder error has no suggestion', () => {
    expect(installErrorText('typhon:install.dest_not_empty: папка установки уже существует и не пуста')).toBe(
      'папка установки уже существует и не пуста',
    );
  });
});

describe('console image refusals', () => {
  it.each([
    ['install.image_not_found', 'в загрузке не найден образ игры для этой консоли'],
    ['install.image_ambiguous', 'в загрузке лежат образы разных игр, выбрать один автоматически нельзя'],
    ['install.image_options_invalid', 'неверные данные для установки образа игры'],
    ['install.image_destination', 'образ игры ставится только в папку Consoles каталога игр'],
    ['library.cover_invalid', 'обложка должна быть адресом http или https'],
    ['library.image_outside_dir', 'образ игры лежит вне папки установки'],
    ['library.already_added', 'эта игра уже добавлена'],
  ])('names %s instead of the fallback', (code, text) => {
    expect(installErrorText(`typhon:${code}: x`)).toBe(text);
  });

  it('keeps a failed image on retry, not on a manual install', () => {
    expect(needsInteractiveInstall('typhon:install.image_ambiguous: x')).toBe(false);
  });
});

describe('image install records', () => {
  it('does not offer a retry for a game that is already in the library', () => {
    expect(canRetryInstall('typhon:library.already_added: эта игра уже добавлена: X')).toBe(false);
    expect(installErrorText('typhon:library.already_added: x')).toBe('эта игра уже добавлена');
  });

  it('keeps the retry for an unreadable image list and names the reason', () => {
    const err = 'typhon:install.image_unreadable: x';
    expect(canRetryInstall(err)).toBe(true);
    expect(installErrorText(err)).toBe('не удалось прочитать список файлов образа игры (.cue или .m3u), файл повреждён');
  });

  it('keeps the retry for ordinary failures', () => {
    expect(canRetryInstall('typhon:install.image_not_found: x')).toBe(true);
    expect(canRetryInstall('')).toBe(true);
  });
});
