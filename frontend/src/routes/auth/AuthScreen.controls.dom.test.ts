import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, login, openForgotPassword } from '../../lib/services/account';
import { authState, authView } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import AuthScreen from './AuthScreen.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/account')>()),
  login: vi.fn(),
  openForgotPassword: vi.fn(async () => {}),
}));

afterEach(() => {
  vi.mocked(login).mockReset();
  vi.mocked(openForgotPassword).mockClear();
  authState.set('bootstrapping');
  authView.set('login');
});

function signInScreen() {
  authState.set('unauthenticated');
  authView.set('login');
  return renderDom(AuthScreen);
}

describe('AuthScreen: links and buttons', () => {
  it('offers the password reset, the support site and the other view as text links the keyboard presses', async () => {
    const { user } = signInScreen();
    const forgot = screen.getByRole('button', { name: msg('tickets.forgotPassword') });
    const cant = screen.getByRole('button', { name: msg('tickets.cantSignIn') });
    const register = screen.getByRole('button', { name: msg('transfers.authCreateAccount') });
    for (const link of [forgot, cant, register]) expect(link.classList.contains('text-link'), link.textContent ?? '').toBe(true);

    forgot.focus();
    await user.keyboard('{Enter}');
    expect(openForgotPassword).toHaveBeenCalledTimes(1);

    register.focus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(msg('transfers.authRegisterTitle'));
  });

  it('signs in with the primary base button, which keeps focus and says it is busy while the request runs', async () => {
    let settle: (err: unknown) => void = () => {};
    vi.mocked(login).mockImplementation(() => new Promise((_resolve, reject) => (settle = reject)));
    const { user } = signInScreen();
    await user.type(screen.getByLabelText(msg('transfers.authIdentifierLabel')), 'player');
    await user.type(screen.getByLabelText(msg('transfers.authPasswordLabel')), 'secret');
    const submit = screen.getByRole('button', { name: msg('transfers.authSignInAction') });
    expect(submit.classList.contains('btn')).toBe(true);
    expect(submit.classList.contains('primary')).toBe(true);

    await user.click(submit);

    expect(submit.getAttribute('aria-busy')).toBe('true');
    expect(submit.hasAttribute('disabled')).toBe(false);
    expect(document.activeElement).toBe(submit);
    expect(submit.textContent).toContain(msg('transfers.authSigningIn'));
    settle(new AccountError('invalid_password'));
    await vi.waitFor(() => expect(submit.hasAttribute('aria-busy')).toBe(false));
  });

  it('continues as a guest through a base button', () => {
    signInScreen();
    const guest = screen.getByRole('button', { name: msg('transfers.authGuestAction') });
    expect(guest.classList.contains('btn')).toBe(true);
  });
});

describe('AuthScreen: messages', () => {
  it('shows a failure of the whole form as a danger notice', async () => {
    vi.mocked(login).mockRejectedValue(new AccountError('server_error'));
    const { container, user } = signInScreen();
    await user.type(screen.getByLabelText(msg('transfers.authIdentifierLabel')), 'player');
    await user.type(screen.getByLabelText(msg('transfers.authPasswordLabel')), 'secret{Enter}');

    const alert = await screen.findByRole('alert');
    const notice = alert.closest('.notice');
    expect(notice?.getAttribute('data-tone')).toBe('danger');
    expect(container.querySelector('p.error')).toBeNull();
  });

  it('keeps a field message out of the field label, tied to the field', async () => {
    vi.mocked(login).mockRejectedValue(new AccountError('invalid_password'));
    const { user } = signInScreen();
    await user.type(screen.getByLabelText(msg('transfers.authIdentifierLabel')), 'player');
    const password = screen.getByLabelText(msg('transfers.authPasswordLabel'));
    await user.type(password, 'secret{Enter}');

    const alert = await screen.findByRole('alert');
    expect(alert.closest('label')).toBeNull();
    expect(password.getAttribute('aria-describedby')).toBe(alert.id);
    expect(screen.getByLabelText(msg('transfers.authPasswordLabel'))).toBe(password);
  });
});
