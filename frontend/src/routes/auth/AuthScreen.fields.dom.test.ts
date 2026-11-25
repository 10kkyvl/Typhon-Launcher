import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { authState, authView } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import AuthScreen from './AuthScreen.svelte';

// The whole account service is a stub: no call from this file reaches a backend; a method a test does
// not set up rejects. Credentials typed here are made-up literals.
const account = vi.hoisted(() => ({
  Bootstrap: vi.fn(),
  Login: vi.fn(),
  Register: vi.fn(),
  ContinueAsGuest: vi.fn(),
  Logout: vi.fn(),
  OpenForgotPassword: vi.fn(),
  OpenSupportSite: vi.fn(),
}));

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/account', () => ({ Service: account }));
vi.mock('../../lib/services/backend', () => ({ inWails: true }));

beforeEach(() => {
  for (const method of Object.values(account)) method.mockImplementation(async () => Promise.reject(new Error('not set up by the test')));
  account.OpenForgotPassword.mockResolvedValue(undefined);
});

afterEach(() => {
  for (const method of Object.values(account)) method.mockReset();
  authState.set('bootstrapping');
  authView.set('login');
});

describe('AuthScreen: a field the server refused', () => {
  it('says it is invalid and is described by its message', async () => {
    authState.set('unauthenticated');
    authView.set('login');
    account.Login.mockRejectedValue(new Error('invalid_password'));
    const { container, user } = renderDom(AuthScreen);
    const identifier = container.querySelector<HTMLInputElement>('input[autocomplete="username"]');
    const password = container.querySelector<HTMLInputElement>('input[type="password"]');
    expect(identifier).toBeInstanceOf(HTMLInputElement);
    expect(password).toBeInstanceOf(HTMLInputElement);
    const field = password as HTMLInputElement;
    expect(field.hasAttribute('aria-invalid')).toBe(false);

    await user.type(identifier as HTMLInputElement, 'player_one');
    await user.type(field, 'fake-pass-1{Enter}');

    await vi.waitFor(() => expect(field.getAttribute('aria-invalid')).toBe('true'));
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent?.trim()).toBeTruthy();
    expect(description(field)).toBe(alert?.textContent?.trim());
    expect((identifier as HTMLInputElement).hasAttribute('aria-invalid')).toBe(false);
  });
});
