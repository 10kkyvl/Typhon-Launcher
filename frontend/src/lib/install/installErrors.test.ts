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

  it('names an interrupted install instead of the fallback', () => {
    expect(installErrorText('typhon:install.interrupted: установка была прервана')).toBe('установка была прервана');
  });
});
