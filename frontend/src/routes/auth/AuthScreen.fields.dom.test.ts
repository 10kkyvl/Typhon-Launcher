import { afterEach, describe, expect, it, vi } from 'vitest';
import { AccountError, login } from '../../lib/services/account';
import { authState, authView } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import AuthScreen from './AuthScreen.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/account')>()),
  login: vi.fn(),
}));

afterEach(() => {
  vi.mocked(login).mockReset();
  authState.set('bootstrapping');
  authView.set('login');
});

describe('AuthScreen: a field the server refused', () => {
  it('says it is invalid and is described by its message', async () => {
    authState.set('unauthenticated');
    authView.set('login');
    vi.mocked(login).mockRejectedValue(new AccountError('invalid_password'));
    const { container, user } = renderDom(AuthScreen);
    const identifier = container.querySelector<HTMLInputElement>('input[autocomplete="username"]');
    const password = container.querySelector<HTMLInputElement>('input[type="password"]');
    expect(identifier).toBeInstanceOf(HTMLInputElement);
    expect(password).toBeInstanceOf(HTMLInputElement);
    const field = password as HTMLInputElement;
    expect(field.hasAttribute('aria-invalid')).toBe(false);

    await user.type(identifier as HTMLInputElement, 'player');
    await user.type(field, 'secret{Enter}');

    await vi.waitFor(() => expect(field.getAttribute('aria-invalid')).toBe('true'));
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent?.trim()).toBeTruthy();
    expect(description(field)).toBe(alert?.textContent?.trim());
    expect((identifier as HTMLInputElement).hasAttribute('aria-invalid')).toBe(false);
  });
});
