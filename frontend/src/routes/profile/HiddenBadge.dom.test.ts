import { render } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { describedControls, hintOnFocus } from '../../lib/testing/hints';
import HiddenBadge from './HiddenBadge.svelte';

vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

beforeEach(() => {
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  document.body.replaceChildren();
});

describe('HiddenBadge', () => {
  it('who the block is hidden from reaches a keyboard and a screen-reader user', async () => {
    render(HiddenBadge, { props: { text: msg('social.statsHiddenFromOthers') } });
    const controls = describedControls(document, msg('social.statsHiddenFromOthers'));
    expect(controls).toHaveLength(1);
    expect(controls[0].textContent?.trim()).toBe(msg('social.hiddenLabel'));
    expect(await hintOnFocus(controls[0])).toBe(msg('social.statsHiddenFromOthers'));
  });
});
