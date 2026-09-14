import { afterEach, expect, it, vi } from 'vitest';
import { applyLanguage } from '../i18n';

vi.mock('./backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/app', () => ({ Service: { SelectExecutable: vi.fn(), SelectGameExecutable: vi.fn(), SetUILanguage: vi.fn(async () => {}) } }));
vi.mock('../../../bindings/typhon/internal/account', () => ({ Service: { PickAvatar: vi.fn(async () => ({ data: '', mime: '' })) } }));
vi.mock('../../../bindings/typhon/internal/download', () => ({ Manager: { AddTorrentSelectFile: vi.fn() } }));
vi.mock('../../../bindings/typhon/internal/relocate', () => ({ Service: { SelectTargetFolder: vi.fn() } }));
vi.mock('../../../bindings/typhon/internal/sources', () => ({ Service: { SelectFeedFile: vi.fn() } }));
vi.mock('../../../bindings/typhon/internal/theme', () => ({ Service: { SelectThemeFile: vi.fn(), SelectExportPath: vi.fn() } }));

import { Service as App } from '../../../bindings/typhon/internal/app';
import { Service as Account } from '../../../bindings/typhon/internal/account';
import { Manager } from '../../../bindings/typhon/internal/download';
import { Service as Relocate } from '../../../bindings/typhon/internal/relocate';
import { Service as Sources } from '../../../bindings/typhon/internal/sources';
import { Service as Theme } from '../../../bindings/typhon/internal/theme';
import { selectExecutable, selectGameExecutable } from './library';
import { pickAvatar } from './account';
import { selectTorrentFile } from './downloads';
import { selectMoveTargetFolder } from './relocate';
import { selectFeedFile } from './sources';
import { selectThemeFile, selectExportPath } from './theme';
import { initNativeLanguage } from './system';

afterEach(() => { vi.unstubAllGlobals(); applyLanguage('ru'); vi.clearAllMocks(); });

it('passes the current resolved language to every native picker on each request', async () => {
  for (const language of ['en', 'ru'] as const) {
    vi.stubGlobal('navigator', { language: `${language}-XX` });
    applyLanguage('system');
    await selectExecutable('Game executable');
    await selectGameExecutable('Game executable', '/games/test', 'game.exe');
    await pickAvatar();
    await selectTorrentFile();
    await selectMoveTargetFolder();
    await selectFeedFile();
    await selectThemeFile();
    await selectExportPath();
    expect(App.SelectExecutable).toHaveBeenLastCalledWith('Game executable', language);
    expect(App.SelectGameExecutable).toHaveBeenLastCalledWith('Game executable', '/games/test', 'game.exe', language);
    for (const call of [Account.PickAvatar, Manager.AddTorrentSelectFile, Relocate.SelectTargetFolder, Sources.SelectFeedFile, Theme.SelectThemeFile, Theme.SelectExportPath]) {
      expect(call).toHaveBeenLastCalledWith(language);
    }
  }
});

it('keeps the native UI language in order when the user switches quickly', async () => {
  applyLanguage('en');
  const unsubscribe = initNativeLanguage();
  try {
    applyLanguage('ru');
    await vi.waitFor(() => expect(App.SetUILanguage).toHaveBeenCalledTimes(2));
    expect(vi.mocked(App.SetUILanguage).mock.calls).toEqual([['en'], ['ru']]);
  } finally {
    unsubscribe();
  }
  applyLanguage('en');
  await Promise.resolve();
  expect(App.SetUILanguage).toHaveBeenCalledTimes(2);
});
