import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

import { msg } from '../../lib/i18n';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { describedControls } from '../../lib/testing/hints';
import { formatCount } from '../../lib/utils/format';
import UserStats from './UserStats.svelte';

/** The text on screen: a hint's words wait in a hidden node until it shows. */
function shownText(element: Element): string {
  const parts: string[] = [];
  const walk = (node: Node) => {
    if (node instanceof Element && node.hasAttribute('hidden')) return;
    if (node.nodeType === Node.TEXT_NODE) parts.push(node.textContent ?? '');
    node.childNodes.forEach(walk);
  };
  walk(element);
  return parts.join(' ').replace(/[ \t\n]+/g, ' ').trim();
}

/** Each figure of the row: the label and what stands above it, a number or the hidden mark. */
function figures(root: HTMLElement): { label: string; shown: string }[] {
  return [...root.querySelectorAll('.stat')].map((stat) => {
    const label = stat.querySelector('.label')?.textContent?.trim() ?? '';
    return { label, shown: shownText(stat).replace(label, '').trim() };
  });
}

beforeEach(() => {
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
});

describe("UserStats: another player's figures", () => {
  it('all shown: games, hours and completed, each a number over its label', () => {
    const { container } = renderDom(UserStats, { stats: { games: 1240, hours: 3600, completed: 12 } });

    expect(figures(container)).toEqual([
      { label: msg('social.statGames'), shown: formatCount(1240) },
      { label: msg('social.statHours'), shown: formatCount(3600) },
      { label: msg('social.statCompleted'), shown: formatCount(12) },
    ]);
    expect(describedControls(container, msg('social.statsHiddenHint'))).toEqual([]);
  });

  it('hours hidden: the hours say they are hidden, with why as a hint the keyboard reaches', () => {
    const { container } = renderDom(UserStats, { stats: { games: 3, hours: null, completed: 1 } });

    expect(figures(container)).toEqual([
      { label: msg('social.statGames'), shown: formatCount(3) },
      { label: msg('social.statHours'), shown: msg('social.hiddenLabel') },
      { label: msg('social.statCompleted'), shown: formatCount(1) },
    ]);
    expect(describedControls(container, msg('social.statsHiddenHint'))).toHaveLength(1);
  });

  it('all hidden: one line says the statistics are hidden, with the same hint, and no number', () => {
    const { container } = renderDom(UserStats, { stats: null });

    expect(container.querySelectorAll('.stat')).toHaveLength(0);
    expect(shownText(container)).toBe(`${msg('social.hiddenLabel')} ${msg('social.statsHiddenLabel')}`);
    expect(describedControls(container, msg('social.statsHiddenHint'))).toHaveLength(1);
  });
});
