import { screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { msg } from '../i18n';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import HiddenBadge from '../../routes/profile/HiddenBadge.svelte';
import StatusBadge from './StatusBadge.svelte';
import source from './StatusBadge.svelte?raw';
import headerSource from '../../routes/profile/ProfileHeader.svelte?raw';

describe('StatusBadge', () => {
  it('draws a dot by default and an icon in its place when given one, hidden from the name', () => {
    renderDom(HiddenBadge, {});
    const badge = screen.getByText(msg('social.hiddenLabel')).closest('.badge') as HTMLElement;
    const mark = badge.querySelector('[data-part="status-badge-icon"]');
    expect(mark?.getAttribute('aria-hidden')).toBe('true');
    expect(mark?.querySelector('svg')).toBeInstanceOf(SVGElement);
    expect(badge.querySelector('.dot')).toBeNull();
    expect(badge.textContent?.trim()).toBe(msg('social.hiddenLabel'));
  });

  it('keeps its dot when it has no icon', () => {
    renderDom(StatusBadge, { label: 'Installed', kind: 'success' });
    const badge = screen.getByText('Installed').closest('.badge') as HTMLElement;
    expect(badge.querySelector('.dot')).not.toBeNull();
    expect(badge.querySelector('[data-part="status-badge-icon"]')).toBeNull();
  });

  // A badge is one word on a plate in a row of them, over a cover or in a list: one line, at its
  // own width, as before. A label of any length (a game's title) asks to wrap: it then stays inside
  // what holds it, breaking between words, and mid-word only where a word alone is wider than the line.
  it('stays on one line unless asked to wrap', async () => {
    const { rerender } = renderDom(StatusBadge, { label: 'Installed', wrap: false });
    const badge = screen.getByText('Installed').closest('.badge') as HTMLElement;
    expect(badge.classList.contains('wrap')).toBe(false);
    await rerender({ label: 'Installed', wrap: true });
    expect(badge.classList.contains('wrap')).toBe(true);

    const css = styleOf(source, 'StatusBadge.svelte');
    expect(ruleBody(css, '.badge')).toMatchObject({ height: '2.4rem', 'white-space': 'nowrap' });
    expect(ruleBody(css, '.badge.wrap')).toMatchObject({ height: 'auto', 'min-height': '2.4rem', 'max-width': '100%', 'white-space': 'normal', 'overflow-wrap': 'break-word' });
    expect(css).not.toMatch(/overflow-wrap:\s*anywhere|word-break/);
    expect(ruleBody(css, '.icon :global(svg)')).toMatchObject({ width: 'var(--badge-icon)', height: 'var(--badge-icon)' });
  });

  it('the badge that holds the title of the game being played wraps', () => {
    const tag = /<StatusBadge\b[^>]*playingIn[^>]*>/.exec(headerSource)?.[0] ?? '';
    expect(tag).toMatch(/\swrap\b/);
  });
});
