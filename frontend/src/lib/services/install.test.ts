import { describe, expect, it, vi } from 'vitest';

vi.mock('../../../bindings/typhon/internal/install', () => ({ Service: {} }));
vi.mock('./backend', () => ({ inWails: false }));

const { installerLikely, offerElevateAhead } = await import('./install');

describe('installerLikely', () => {
  it('sees nothing in an empty download', () => {
    expect(installerLikely([])).toBe(false);
  });

  it('sees an installer next to data files', () => {
    expect(installerLikely(['data.bin', 'setup.exe'])).toBe(true);
    expect(installerLikely(['Game.msi'])).toBe(true);
  });

  it('ignores case', () => {
    expect(installerLikely(['SETUP.EXE'])).toBe(true);
  });

  it('leaves portable builds and archives alone', () => {
    expect(installerLikely(['Game/Game.bin', 'Game/data/pak0.pak'])).toBe(false);
    expect(installerLikely(['Game.part1.rar', 'Game.iso'])).toBe(false);
  });
});

describe('offerElevateAhead', () => {
  const installer = ['setup.exe'];
  const portable = ['Game/Game.bin'];

  it('offers only when rights can actually be asked for', () => {
    expect(offerElevateAhead({ elevationSupported: false, autoInstall: true, paths: installer })).toBe(false);
    expect(offerElevateAhead({ elevationSupported: true, autoInstall: true, paths: installer })).toBe(true);
  });

  it('stays hidden while nothing will be installed automatically', () => {
    expect(offerElevateAhead({ elevationSupported: true, autoInstall: false, paths: installer })).toBe(false);
  });

  it('stays hidden for a payload that needs no rights', () => {
    expect(offerElevateAhead({ elevationSupported: true, autoInstall: true, paths: portable })).toBe(false);
  });
});
