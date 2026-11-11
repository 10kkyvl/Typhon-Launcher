import { fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { stageLabel } from '../relocate/moveText';
import type { MoveJob } from '../services/relocate';
import { moves } from '../stores/relocate';
import { renderDom } from '../testing/dom';
import { bytesSize } from '../utils/format';
import ActivityDock from './ActivityDock.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const job: MoveJob = {
  id: 'm1',
  scope: 'game',
  stage: 'copy',
  gameId: 'g1',
  title: 'Hades',
  source: '/a',
  target: '/b',
  staging: '/b/.staging',
  renamed: false,
  totalBytes: 100,
  copiedBytes: 42,
  phase: '',
  currentFile: '',
  startedAt: '',
  updatedAt: '',
} as MoveJob;

afterEach(() => moves.set([]));

describe('ActivityDock rows', () => {
  // The row is a button: its content is its name, and a bar in it would add its value to that name.
  it('names a row from its text alone, with the bar hidden from assistive tech', async () => {
    moves.set([job]);
    const { container, queryAllByRole, getByRole } = renderDom(ActivityDock);
    await fireEvent.focusIn(container.querySelector('.dock') as HTMLElement);

    const row = container.querySelector<HTMLElement>('.row');
    let name = '';
    getByRole('button', {
      name: (computed, element) => {
        if (element === row) name = computed;
        return element === row;
      },
    });
    expect(name).toBe(['Hades', '42%', stageLabel('copy'), `${bytesSize(42)} / ${bytesSize(100)}`].join(' '));
    expect(queryAllByRole('progressbar')).toEqual([]);
    expect(row?.querySelector('.track')?.getAttribute('aria-hidden')).toBe('true');
  });
});
