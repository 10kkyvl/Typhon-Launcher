import { describe, expect, it } from 'vitest';
import { defaultWrapper, describeImports, presetsFor, problemActions, problemText, progressView } from './compatMode';

const size = (n: number) => `${n}b`;

describe('describeImports', () => {
  it('names the highest DirectX and keeps the dll list', () => {
    expect(describeImports(['ddraw', 'd3d8'])).toBe('DirectX 8 (ddraw, d3d8)');
  });

  it('merges d3d10 variants into one name', () => {
    expect(describeImports(['d3d10_1', 'd3d10'])).toBe('DirectX 10 (d3d10_1, d3d10)');
  });

  it('keeps DirectDraw when it is the only DirectX', () => {
    expect(describeImports(['ddraw'])).toBe('DirectDraw (ddraw)');
  });

  it('lists OpenGL and Glide next to DirectX', () => {
    expect(describeImports(['d3d9', 'opengl32', 'glide2x'])).toBe('DirectX 9, OpenGL, Glide (d3d9, opengl32, glide2x)');
  });

  it('is empty for nothing recognisable', () => {
    expect(describeImports([])).toBe('');
    expect(describeImports(['kernel32'])).toBe('');
  });

  it('ignores case and the dll suffix', () => {
    expect(describeImports(['D3D8.dll'])).toBe('DirectX 8 (d3d8)');
  });
});

describe('defaultWrapper', () => {
  it('follows the suggestion', () => {
    expect(defaultWrapper({ suggested: 'dxvk' })).toBe('dxvk');
    expect(defaultWrapper({ suggested: 'dgvoodoo' })).toBe('dgvoodoo');
  });

  it('falls back to dgVoodoo2 without one', () => {
    expect(defaultWrapper({ suggested: '' })).toBe('dgvoodoo');
  });
});

describe('presetsFor', () => {
  it('gives DXVK only the default', () => {
    expect(presetsFor('dxvk')).toEqual(['default']);
    expect(presetsFor('dgvoodoo')).toEqual(['default', '1080p', 'native']);
  });
});

describe('progressView', () => {
  it('shows bytes and a fraction while downloading', () => {
    const view = progressView({ gameId: 'g', stage: 'download', done: 5, total: 20 }, size);
    expect(view.fraction).toBe(0.25);
    expect(view.text).toContain('5b');
    expect(view.text).toContain('20b');
  });

  it('has no fraction when the total is unknown', () => {
    const view = progressView({ gameId: 'g', stage: 'download', done: 5, total: 0 }, size);
    expect(view.fraction).toBeNull();
    expect(view.text).toContain('5b');
  });

  it('counts files while extracting and applying', () => {
    const extract = progressView({ gameId: 'g', stage: 'extract', done: 2, total: 8 }, size);
    const apply = progressView({ gameId: 'g', stage: 'apply', done: 1, total: 3 }, size);
    expect(extract.text).not.toBe(apply.text);
    expect(extract.fraction).toBe(0.25);
    expect(apply.text).toContain('1');
  });

  it('has a text without numbers when the total is unknown', () => {
    const view = progressView({ gameId: 'g', stage: 'extract', done: 0, total: 0 }, size);
    expect(view.fraction).toBeNull();
    expect(view.text).not.toBe('');
  });

  it('clamps a fraction that overshoots', () => {
    expect(progressView({ gameId: 'g', stage: 'apply', done: 9, total: 3 }, size).fraction).toBe(1);
  });
});

describe('problemText', () => {
  it('names the changed file', () => {
    expect(problemText('tampered', 'd3d9.dll')).toContain('d3d9.dll');
  });

  it('accepts a prefixed code', () => {
    expect(problemText('wrappers.backup_corrupt')).toBe(problemText('backup_corrupt'));
    expect(problemText('backup_corrupt')).not.toBe(problemText('mystery'));
  });

  it('does not invent a reason for an unknown code', () => {
    expect(problemText('mystery')).toBe(problemText('other'));
  });
});

describe('problemActions', () => {
  it('offers both ways out of a tamper', () => {
    expect(problemActions('wrappers.tampered')).toEqual({ revert: true, keepChanged: true, dirMissing: false });
    expect(problemActions('tampered').keepChanged).toBe(true);
  });

  it('offers only the plain revert for other problems', () => {
    expect(problemActions('wrappers.backup_corrupt')).toEqual({ revert: true, keepChanged: false, dirMissing: false });
    expect(problemActions('needs_repair').keepChanged).toBe(false);
  });

  it('offers nothing while the game folder is missing', () => {
    expect(problemActions('wrappers.game_dir_missing')).toEqual({ revert: false, keepChanged: false, dirMissing: true });
  });
});
