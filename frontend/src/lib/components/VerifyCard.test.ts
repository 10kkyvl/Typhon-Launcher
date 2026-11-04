import { render } from 'svelte/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
vi.mock('../stores/updates', () => ({ createManifest: vi.fn(), repair: vi.fn(), verify: vi.fn() }));
import VerifyCard from './VerifyCard.svelte';
import type { VerifyState } from '../services/updates';
import { locale } from '../i18n';

function damaged(method: 'manifest' | 'torrent'): VerifyState {
  return {
    gameId: 'g',
    method,
    checkedAt: '2026-09-14T15:10:50Z',
    ratio: 0.99,
    totalBytes: 1000,
    okBytes: 990,
    corruptedPieces: 1,
    missingFiles: 0,
    unreadableFiles: 0,
  } as unknown as VerifyState;
}

beforeEach(() => locale.set('ru'));

// A manifest check compares whole files; only a torrent check counts pieces.
describe('verify card damage unit', () => {
  it('counts files for a manifest check', () => {
    const html = render(VerifyCard, { props: { gameId: 'g', state: damaged('manifest'), running: false } }).body;
    expect(html).toContain('1 файл');
    expect(html).not.toContain('1 блок');
  });

  it('counts blocks for a torrent check', () => {
    const html = render(VerifyCard, { props: { gameId: 'g', state: damaged('torrent'), running: false } }).body;
    expect(html).toContain('1 блок');
  });
});
