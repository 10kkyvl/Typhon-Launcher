import { describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { locale } from '../i18n';
import Toggle from '../components/Toggle.svelte';
import { renderDom } from './dom';

describe('DOM test setup', () => {
  it('mounts a component into the document and clicks it', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: 'Звук', onchange });

    await user.click(getByRole('switch', { name: 'Звук' }));

    expect(onchange).toHaveBeenCalledWith(true);
    expect(document.body.querySelectorAll('[role="switch"]')).toHaveLength(1);
    locale.set('en');
    vi.useFakeTimers();
  });

  it('starts the next test from an empty document, in Russian, on the real clock', () => {
    expect(document.body.querySelector('[role="switch"]')).toBeNull();
    expect(get(locale)).toBe('ru');
    expect(vi.isFakeTimers()).toBe(false);
  });

  it('lets the user act while the clock is fake', async () => {
    vi.useFakeTimers();
    const { getByRole, user } = renderDom(Toggle, { label: 'Звук' });

    await user.click(getByRole('switch'));
    vi.advanceTimersByTime(1000);

    expect(getByRole('switch').getAttribute('aria-checked')).toBe('true');
  });
});
