import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { Update } from '../services/updates';
import { renderDom } from '../testing/dom';
import UpdateCard from './UpdateCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

function update(patch: Partial<Update> = {}): Update {
  return {
    gameId: 'g', title: 'Game', state: 'update_available', planning: false, progress: 0, canRollback: false, checkedAt: '',
    availability: {
      available: true, kind: 'update', gameId: 'g', installedReleaseId: 'r1', targetReleaseId: 'r2', installedVersion: '1', targetVersion: '2',
      confidence: 1, strategy: 'full_release', estimatedDownloadBytes: 10, requiresFullInstall: true, patchCount: 0, targetSize: 10, reason: '',
    },
    ...patch,
  } as Update;
}

const plan = {
  strategy: 'full_release', downloadBytes: 10, reusedBytes: 0, requiredDiskBytes: 20, installedVersion: '1', targetVersion: '2',
  backupAvailable: true, rollbackAvailable: true, steps: [{ label: 'Download', bytes: 10 }],
};

describe('UpdateCard', () => {
  it('shows the plan through a Details disclosure that says what it opens', async () => {
    const { user } = renderDom(UpdateCard, { update: update({ plan } as Partial<Update>), running: false });
    const details = screen.getByRole('button', { name: msg('ui.details') });
    expect(details.getAttribute('aria-expanded')).toBe('false');
    expect(details.hasAttribute('aria-pressed')).toBe(false);
    details.focus();

    await user.keyboard('{Enter}');

    expect(details.getAttribute('aria-expanded')).toBe('true');
    const panel = document.getElementById(details.getAttribute('aria-controls') ?? '');
    expect(panel?.textContent).toContain(msg('ui.spaceNeeded'));
  });

  it('shows the size being calculated as the base spinner named by that', () => {
    renderDom(UpdateCard, { update: update({ planning: true }), running: false });
    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('ui.calculatingDownloadSize'));
  });

  it('states a running game and a failed update in notices of the card', () => {
    renderDom(UpdateCard, { update: update({ error: 'disk_full' } as Partial<Update>), running: true });
    expect(screen.getByText(msg('ui.gameRunningBeforeUpdate')).closest('.notice')).toBeTruthy();
    expect(document.querySelector('.notice[data-tone="danger"]')).toBeTruthy();
    expect(document.querySelector('p.error')).toBeNull();
  });
});
