import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { locale } from '../i18n';
import { en } from '../i18n/catalog/en';
import { ru } from '../i18n/catalog/ru';
import {
  effectiveGameMode,
  gameModeErrorText,
  inheritLabel,
  overrideState,
  withOverride,
} from './gamemode';

afterEach(() => locale.set('ru'));

const off = { highPerformance: false, hideLauncher: false, raisePriority: false };

describe('overrideState', () => {
  it('reads a missing key as inherit and keeps false apart from it', () => {
    expect(overrideState(undefined, 'raisePriority')).toBe('inherit');
    expect(overrideState(null, 'raisePriority')).toBe('inherit');
    expect(overrideState({}, 'raisePriority')).toBe('inherit');
    expect(overrideState({ raisePriority: null }, 'raisePriority')).toBe('inherit');
    expect(overrideState({ raisePriority: true }, 'raisePriority')).toBe('on');
    expect(overrideState({ raisePriority: false }, 'raisePriority')).toBe('off');
  });
});

describe('withOverride', () => {
  it('changes only the chosen switch', () => {
    expect(withOverride({ hideLauncher: true }, 'raisePriority', 'off')).toEqual({
      hideLauncher: true,
      raisePriority: false,
    });
  });

  it('drops the key when going back to inherit and ends up empty when nothing is left', () => {
    expect(withOverride({ raisePriority: true, hideLauncher: false }, 'raisePriority', 'inherit')).toEqual({
      hideLauncher: false,
    });
    expect(withOverride({ raisePriority: true }, 'raisePriority', 'inherit')).toEqual({});
    expect(withOverride(null, 'highPerformance', 'inherit')).toEqual({});
  });

  it('never sends null for an untouched switch', () => {
    const next = withOverride({ highPerformance: null }, 'hideLauncher', 'on');
    expect(Object.keys(next)).toEqual(['hideLauncher']);
  });
});

describe('effectiveGameMode', () => {
  it('follows the global value only while inheriting', () => {
    const global = { ...off, raisePriority: true };
    expect(effectiveGameMode(global, undefined, 'raisePriority')).toBe(true);
    expect(effectiveGameMode(global, { raisePriority: false }, 'raisePriority')).toBe(false);
    expect(effectiveGameMode(off, { raisePriority: true }, 'raisePriority')).toBe(true);
    expect(effectiveGameMode(undefined, undefined, 'raisePriority')).toBe(false);
  });
});

describe('inheritLabel', () => {
  it('shows the inherited value in both languages', () => {
    expect(inheritLabel(true)).toBe('Как в настройках (вкл)');
    expect(inheritLabel(false)).toBe('Как в настройках (выкл)');
    locale.set('en');
    expect(inheritLabel(true)).toBe('As in settings (on)');
  });
});

describe('gameModeErrorText', () => {
  const codes = Object.keys(ru).filter((key) => key.startsWith('gamemode.'));

  it('has a message in both languages for every code', () => {
    expect(codes.length).toBe(9);
    for (const code of codes) {
      expect(Object.prototype.hasOwnProperty.call(en, code)).toBe(true);
      expect(gameModeErrorText(code)).toBe(ru[code as keyof typeof ru]);
    }
  });

  it('falls back to the general message for an unknown or foreign code', () => {
    expect(gameModeErrorText('gamemode.something_new')).toBe(ru['gamemode.failed']);
    expect(gameModeErrorText('library.game_not_found')).toBe(ru['gamemode.failed']);
    expect(gameModeErrorText('')).toBe(ru['gamemode.failed']);
  });

  it('speaks English when the language is English', () => {
    locale.set('en');
    expect(gameModeErrorText('gamemode.hide_needs_tray')).toBe(en['gamemode.hide_needs_tray']);
  });
});
