import { describe, expect, it } from 'vitest';
import { installErrorText, needsInteractiveInstall } from './installErrors';

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
