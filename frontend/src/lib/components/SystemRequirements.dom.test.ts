import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { checkSystemRequirements, type SysreqReport } from '../services/sysreq';
import { renderDom } from '../testing/dom';
import SystemRequirements from './SystemRequirements.svelte';

vi.mock('../services/sysreq', () => ({ checkSystemRequirements: vi.fn() }));

const report: SysreqReport = {
  verdict: 'good',
  reason: '',
  checks: [
    {
      kind: 'ram',
      required: '8 GB',
      recommended: '16 GB',
      have: { text: '', cores: 0, bits: 0, bytes: 32 * 1024 ** 3, free: null, directX: 0, installed: false },
      status: 'ok',
    },
  ],
};

afterEach(() => vi.restoreAllMocks());

describe('SystemRequirements: the list of checks', () => {
  it('opens under a disclosure button tied to the list', async () => {
    vi.mocked(checkSystemRequirements).mockResolvedValue(report);
    const { user } = renderDom(SystemRequirements, { gameId: 'sysreq-disclosure', installPath: '/games' });
    const toggle = await screen.findByRole('button', { name: msg('sysreq.show') });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(toggle.hasAttribute('aria-pressed')).toBe(false);
    toggle.focus();

    await user.keyboard('{Enter}');

    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    const list = document.getElementById(toggle.getAttribute('aria-controls') ?? '');
    expect(list?.tagName).toBe('UL');
    expect(list?.textContent).toContain(msg('sysreq.required'));
  });
});
