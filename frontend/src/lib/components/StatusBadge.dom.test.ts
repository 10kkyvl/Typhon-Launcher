import { screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { msg } from '../i18n';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import HiddenBadge from '../../routes/profile/HiddenBadge.svelte';
import StatusBadge from './StatusBadge.svelte';
import source from './StatusBadge.svelte?raw';

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

  // A label of any length (a game's title) stays inside what holds the badge: it wraps, so all of it
  // stays in sight with no tooltip or focus stop of its own (a badge may have its own hint).
  it('wraps a long label inside its container rather than running out of it', () => {
    const css = styleOf(source, 'StatusBadge.svelte');
    const badge = ruleBody(css, '.badge');
    expect(badge).toMatchObject({ 'max-width': '100%', 'white-space': 'normal', 'overflow-wrap': 'anywhere', 'min-height': '2.4rem' });
    expect(badge.height).toBeUndefined();
    expect(css).not.toMatch(/white-space:\s*nowrap/);
    expect(ruleBody(css, '.icon :global(svg)')).toMatchObject({ width: 'var(--badge-icon)', height: 'var(--badge-icon)' });
  });
});
