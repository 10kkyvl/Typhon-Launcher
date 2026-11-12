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

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
// Read before the press; the getter answers the words of a message that came in since, '' until then.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

afterEach(() => {
  vi.mocked(checkSystemRequirements).mockReset();
  vi.restoreAllMocks();
});

describe('SystemRequirements: loading and retrying', () => {
  it('shows the base spinner while it checks', async () => {
    vi.mocked(checkSystemRequirements).mockReturnValue(new Promise(() => {}));
    renderDom(SystemRequirements, { gameId: 'sysreq-loading', installPath: '/games' });

    const status = screen.getByRole('status');

    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('sysreq.loading'));
  });

  it('keeps the failure notice and the focus on its busy Retry until the answer, then focuses the checks toggle', async () => {
    let answer!: (value: SysreqReport) => void;
    vi.mocked(checkSystemRequirements)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = renderDom(SystemRequirements, { gameId: 'sysreq-retry', installPath: '/games' });
    const retry = await screen.findByRole('button', { name: msg('sysreq.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    expect(notice).toBeInstanceOf(HTMLElement);
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer(report);
    const toggle = await screen.findByRole('button', { name: msg('sysreq.show') });
    expect(notice.isConnected).toBe(false);
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it('keeps the failure notice when the retry fails again, and says it again', async () => {
    vi.mocked(checkSystemRequirements).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(SystemRequirements, { gameId: 'sysreq-retry-again', installPath: '/games' });
    const retry = await screen.findByRole('button', { name: msg('sysreq.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('sysreq.failed')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('SystemRequirements: the list of checks', () => {
  it('opens under a disclosure button tied to the list', async () => {
    vi.mocked(checkSystemRequirements).mockResolvedValue(report);
    const { user } = renderDom(SystemRequirements, { gameId: 'sysreq-disclosure', installPath: '/games' });
    const toggle = await screen.findByRole('button', { name: msg('sysreq.show') });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(toggle.hasAttribute('aria-pressed')).toBe(false);
    const list = document.getElementById(toggle.getAttribute('aria-controls') ?? '');
    expect(list?.tagName).toBe('UL');
    expect(list?.hidden).toBe(true);
    toggle.focus();

    await user.keyboard('{Enter}');

    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(list?.isConnected).toBe(true);
    expect(list?.hidden).toBe(false);
    expect(list?.textContent).toContain(msg('sysreq.required'));

    await user.keyboard('{Enter}');
    expect(list?.hidden).toBe(true);
  });
});
