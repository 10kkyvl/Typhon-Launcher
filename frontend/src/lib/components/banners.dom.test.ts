import { render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { user } from '../testing/dom';
import OfflineBanner from './OfflineBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

describe('the lost connection banner', () => {
  beforeEach(() => {
    locale.set('en');
  });

  it('is a warning notice with its two actions, and its close takes it away', async () => {
    render(OfflineBanner);

    const notice = screen.getByRole('status');
    expect(notice.getAttribute('data-tone')).toBe('warning');
    const buttons = within(notice).getAllByRole('button').map((button) => button.textContent?.trim());
    expect(buttons).toEqual(['Service status', 'Retry', '']);

    await user().click(within(notice).getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('status')).toBeNull();
  });
});
