import { describe, expect, it, vi } from 'vitest';
import { platformBadge } from '../library/emulation';
import { renderDom } from '../testing/dom';
import ConsoleTag from './ConsoleTag.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

describe('ConsoleTag', () => {
  it('names the platform with the base badge', () => {
    const { container } = renderDom(ConsoleTag, { platform: 'ps2' });
    const badge = container.querySelector('.badge');
    expect(badge?.classList.contains('neutral')).toBe(true);
    expect(badge?.querySelector('.dot')).toBeNull();
    expect(badge?.textContent?.trim()).toBe(platformBadge('ps2'));
  });
});
