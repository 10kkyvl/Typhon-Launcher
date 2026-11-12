import { render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../../i18n';
import { igdbIdsOf } from '../../services/profile';
import { user } from '../../testing/dom';
import GamePicker from './GamePicker.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));
vi.mock('../../services/profile', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/profile')>()),
  igdbIdsOf: vi.fn(),
}));

describe('the game picker', () => {
  beforeEach(() => {
    locale.set('en');
    vi.mocked(igdbIdsOf).mockReset();
  });

  // Every failed load on a page carries its Retry in the notice's action, outside the live region.
  it('keeps the retry of a failed load inside its notice, and the notice goes once it loads', async () => {
    vi.mocked(igdbIdsOf).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({});
    render(GamePicker, { props: { chosen: [], onpick: () => {} } });

    const alert = await screen.findByRole('alert');
    const notice = alert.closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: 'Retry' });
    expect(alert.contains(retry)).toBe(false);

    await user().click(retry);
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(igdbIdsOf).toHaveBeenCalledTimes(2);
  });
});
