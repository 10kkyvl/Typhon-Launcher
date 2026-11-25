import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { authReason, authState, authView, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import AuthScreen from './AuthScreen.svelte';

// The whole account service is a stub: no call from this file reaches a backend. A method a test does
// not set up rejects, so an unexpected call fails the test instead of going anywhere. Every credential
// typed here is a made-up literal.
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

const PLAYER = {
  id: 'u-1',
  username: 'player_one',
  displayName: 'Player One',
  email: 'player@example.test',
  avatarUrl: '',
  bio: '',
  createdAt: '2026-01-01T00:00:00Z',
};

beforeEach(() => {
  for (const method of Object.values(account)) method.mockImplementation(async () => Promise.reject(new Error('not set up by the test')));
});

afterEach(() => {
  for (const method of Object.values(account)) method.mockReset();
  currentUser.set(null);
  authReason.set('');
  authState.set('bootstrapping');
  authView.set('login');
});

function screenIn(view: 'login' | 'register') {
  authState.set('unauthenticated');
  authView.set(view);
  return renderDom(AuthScreen);
}

const field = (name: string) => screen.getByLabelText(name) as HTMLInputElement;

// What a field asks of the browser and of a password manager.
function shape(input: HTMLInputElement) {
  return {
    type: input.type,
    autocomplete: input.getAttribute('autocomplete'),
    required: input.required,
    maxLength: input.maxLength,
  };
}

describe('AuthScreen: the two forms', () => {
  it('signs in with a login and a password, both required', () => {
    screenIn('login');
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(msg('transfers.authLoginTitle'));
    expect(shape(field(msg('transfers.authIdentifierLabel')))).toEqual({ type: 'text', autocomplete: 'username', required: true, maxLength: -1 });
    expect(shape(field(msg('transfers.authPasswordLabel')))).toEqual({ type: 'password', autocomplete: 'current-password', required: true, maxLength: -1 });
    expect(screen.queryByLabelText(msg('common.email'))).toBeNull();
    expect(screen.getByRole('button', { name: msg('transfers.authSignInAction') }).getAttribute('type')).toBe('submit');
  });

  it('signs up with an e-mail, a username, a display name and a new password, all required', () => {
    screenIn('register');
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(msg('transfers.authRegisterTitle'));
    expect(shape(field(msg('common.email')))).toEqual({ type: 'email', autocomplete: 'email', required: true, maxLength: -1 });
    expect(shape(field(msg('transfers.authUsernameLabel')))).toEqual({ type: 'text', autocomplete: 'username', required: true, maxLength: 24 });
    expect(shape(field(msg('transfers.authDisplayNameLabel')))).toEqual({ type: 'text', autocomplete: 'nickname', required: true, maxLength: 32 });
    expect(shape(field(msg('transfers.authPasswordLabel')))).toEqual({ type: 'password', autocomplete: 'new-password', required: true, maxLength: -1 });
    expect(screen.queryByLabelText(msg('transfers.authIdentifierLabel'))).toBeNull();
    expect(screen.queryByRole('button', { name: msg('tickets.forgotPassword') })).toBeNull();
    expect(screen.queryByRole('button', { name: msg('tickets.cantSignIn') })).toBeNull();
  });

  it('keeps what was typed in the other form across a switch, except the password', async () => {
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1');

    await user.click(screen.getByRole('button', { name: msg('transfers.authCreateAccount') }));
    expect(field(msg('transfers.authPasswordLabel')).value).toBe('');
    await user.type(field(msg('common.email')), 'player@example.test');

    await user.click(screen.getByRole('button', { name: msg('transfers.authSignInAction') }));
    expect(field(msg('transfers.authIdentifierLabel')).value).toBe('player_one');
    expect(field(msg('transfers.authPasswordLabel')).value).toBe('');

    await user.click(screen.getByRole('button', { name: msg('transfers.authCreateAccount') }));
    expect(field(msg('common.email')).value).toBe('player@example.test');
  });
});

describe('AuthScreen: what a submit sends', () => {
  it('signs in by Enter in a field with what was typed, and lets the user in', async () => {
    account.Login.mockResolvedValue(PLAYER);
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');

    await vi.waitFor(() => expect(get(authState)).toBe('authenticated'));
    expect(account.Login).toHaveBeenCalledTimes(1);
    expect(account.Login).toHaveBeenCalledWith({ emailOrUsername: 'player_one', password: 'fake-pass-1' });
    expect(get(currentUser)?.id).toBe('u-1');
  });

  it('signs up by the button with what was typed', async () => {
    account.Register.mockResolvedValue(PLAYER);
    const { user } = screenIn('register');
    await user.type(field(msg('common.email')), 'player@example.test');
    await user.type(field(msg('transfers.authUsernameLabel')), 'player_one');
    await user.type(field(msg('transfers.authDisplayNameLabel')), 'Player One');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-2');
    await user.click(screen.getByRole('button', { name: msg('transfers.authCreateAccount') }));

    await vi.waitFor(() => expect(get(authState)).toBe('authenticated'));
    expect(account.Register).toHaveBeenCalledWith({ email: 'player@example.test', username: 'player_one', displayName: 'Player One', password: 'fake-pass-2' });
  });

  it('submits once: a second Enter while the answer is pending sends nothing', async () => {
    let answer: (user: typeof PLAYER) => void = () => {};
    account.Login.mockImplementation(() => new Promise((resolve) => (answer = resolve)));
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    const password = field(msg('transfers.authPasswordLabel'));
    await user.type(password, 'fake-pass-1{Enter}');
    await user.keyboard('{Enter}');
    await user.click(screen.getByRole('button', { name: msg('transfers.authSigningIn') }));

    expect(account.Login).toHaveBeenCalledTimes(1);
    answer(PLAYER);
    await vi.waitFor(() => expect(get(authState)).toBe('authenticated'));
  });

  it('continues as a guest with no credentials', async () => {
    account.ContinueAsGuest.mockResolvedValue(undefined);
    const { user } = screenIn('login');
    await user.click(screen.getByRole('button', { name: msg('transfers.authGuestAction') }));

    await vi.waitFor(() => expect(get(authState)).toBe('guest'));
    expect(account.ContinueAsGuest).toHaveBeenCalledWith();
    expect(account.Login).not.toHaveBeenCalled();
  });

  it('opens the password reset and the support site from the sign-in form', async () => {
    account.OpenForgotPassword.mockResolvedValue(undefined);
    account.OpenSupportSite.mockResolvedValue(undefined);
    const { user } = screenIn('login');
    await user.click(screen.getByRole('button', { name: msg('tickets.forgotPassword') }));
    await user.click(screen.getByRole('button', { name: msg('tickets.cantSignIn') }));

    expect(account.OpenForgotPassword).toHaveBeenCalledTimes(1);
    expect(account.OpenSupportSite).toHaveBeenCalledTimes(1);
  });
});

describe('AuthScreen: a refused submit', () => {
  it('says a failure of the whole form once, in a danger notice, marks no field and keeps focus on the pressed button', async () => {
    account.Login.mockRejectedValue(new Error('invalid_credentials'));
    const { container, user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1');
    const submit = screen.getByRole('button', { name: msg('transfers.authSignInAction') });
    await user.click(submit);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.accountErrorInvalidCredentials'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(container.querySelectorAll('[aria-invalid="true"]')).toHaveLength(0);
    expect(document.activeElement).toBe(submit);
  });

  it('says a reply outside the error contract as the form\'s own failure, never its raw text', async () => {
    account.Login.mockRejectedValue(new Error('pq: relation "users" does not exist'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('transfers.authGenericError'));
    expect(document.body.textContent).not.toContain('relation');
    error.mockRestore();
  });

  it.each([
    ['email_taken', 'common.email', 'state.accountErrorEmailTaken'],
    ['invalid_email', 'common.email', 'state.accountErrorInvalidEmail'],
    ['username_taken', 'transfers.authUsernameLabel', 'state.accountErrorUsernameTaken'],
    ['invalid_display_name', 'transfers.authDisplayNameLabel', 'state.accountErrorInvalidDisplayName'],
    ['invalid_password', 'transfers.authPasswordLabel', 'state.accountErrorInvalidPassword'],
  ] as const)('ties a sign-up refusal %s to the field it is about', async (code, label, text) => {
    account.Register.mockRejectedValue(new Error(code));
    const { container, user } = screenIn('register');
    await user.type(field(msg('common.email')), 'player@example.test');
    await user.type(field(msg('transfers.authUsernameLabel')), 'player_one');
    await user.type(field(msg('transfers.authDisplayNameLabel')), 'Player One');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-2{Enter}');

    const input = field(msg(label));
    await vi.waitFor(() => expect(input.getAttribute('aria-invalid')).toBe('true'));
    expect(description(input)).toBe(msg(text));
    expect(container.querySelectorAll('[aria-invalid="true"]')).toHaveLength(1);
    expect(container.querySelector('.notice')).toBeNull();
  });

  it('ties a username refusal on the sign-in form to the login field', async () => {
    account.Login.mockRejectedValue(new Error('invalid_username'));
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'x');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');

    const login = field(msg('transfers.authIdentifierLabel'));
    await vi.waitFor(() => expect(login.getAttribute('aria-invalid')).toBe('true'));
    expect(description(login)).toBe(msg('state.accountErrorInvalidUsername'));
  });

  it('clears the message when the next submit starts', async () => {
    account.Login.mockRejectedValueOnce(new Error('invalid_credentials'));
    let answer: (user: typeof PLAYER) => void = () => {};
    account.Login.mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)));
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');
    await screen.findByRole('alert');

    await user.click(screen.getByRole('button', { name: msg('transfers.authSignInAction') }));
    expect(screen.queryByRole('alert')).toBeNull();
    answer(PLAYER);
    await vi.waitFor(() => expect(get(authState)).toBe('authenticated'));
  });

  it('says a refused guest entry in the form', async () => {
    account.ContinueAsGuest.mockRejectedValue(new Error('network_error'));
    const { user } = screenIn('login');
    const guest = screen.getByRole('button', { name: msg('transfers.authGuestAction') });
    await user.click(guest);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.accountErrorNetwork'));
    expect(document.activeElement).toBe(guest);
    expect(get(authState)).toBe('unauthenticated');
  });
});

describe('AuthScreen: the account server out of reach', () => {
  function unavailable(reason: string) {
    authReason.set(reason);
    authState.set('unavailable');
    return renderDom(AuthScreen);
  }

  it.each([
    ['network_error', 'state.accountErrorNetwork'],
    ['', 'transfers.authServerUnavailable'],
    ['a_code_this_build_does_not_know', 'transfers.authServerUnavailable'],
  ] as const)('says why by the reason %j, from the table, with no form', (reason, text) => {
    unavailable(reason);
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(msg('transfers.authOfflineTitle'));
    // The heading and the network reason can be the same words: the reason is the paragraph.
    expect(screen.getAllByText(msg(text)).filter((node) => node.tagName === 'P')).toHaveLength(1);
    expect(screen.getByText(msg('transfers.authOfflineNote'))).toBeTruthy();
    expect(screen.queryByRole('textbox')).toBeNull();
    expect(screen.queryByRole('button', { name: msg('transfers.authGuestAction') })).toBeNull();
  });

  it('asks the server again on Retry and shows the sign-in form once it answers', async () => {
    account.Bootstrap.mockResolvedValue({ status: 'unauthenticated', user: null, reason: '' });
    const { user } = unavailable('network_error');
    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    expect(account.Bootstrap).toHaveBeenCalledTimes(1);
    expect(await screen.findByLabelText(msg('transfers.authIdentifierLabel'))).toBeTruthy();
    expect(get(authState)).toBe('unauthenticated');
  });

  it('keeps Retry under focus and busy while the server is asked, then hands focus to the login field', async () => {
    let answer: (state: unknown) => void = () => {};
    account.Bootstrap.mockImplementation(() => new Promise((resolve) => (answer = resolve)));
    const { user } = unavailable('network_error');
    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();
    await user.keyboard('{Enter}');
    const asked = { connected: retry.isConnected, busy: retry.getAttribute('aria-busy'), focused: document.activeElement === retry };
    // The store keeps one bootstrap in flight for every caller: it is answered before any assertion,
    // so a failure here leaves none pending for the next test.
    answer({ status: 'unauthenticated', user: null, reason: '' });

    expect(asked).toEqual({ connected: true, busy: 'true', focused: true });
    const login = await screen.findByLabelText(msg('transfers.authIdentifierLabel'));
    await vi.waitFor(() => expect(document.activeElement).toBe(login));
  });

  it('keeps focus on the same Retry when it fails again', async () => {
    account.Bootstrap.mockRejectedValue(new Error('network_error'));
    const { user } = unavailable('network_error');
    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();
    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(account.Bootstrap).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });

  it('stays on the out-of-reach state with the new reason when Retry fails again', async () => {
    account.Bootstrap.mockRejectedValue(new Error('rate_limited'));
    const { user } = unavailable('network_error');
    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    expect(await screen.findByText(msg('state.accountErrorRateLimited'))).toBeTruthy();
    expect(get(authState)).toBe('unavailable');
  });
});

describe('AuthScreen: a password the user can see', () => {
  const reveal = () => screen.getByRole('button', { name: msg('transfers.authShowPassword') });

  it('is hidden at first, shown and hidden again by a named toggle that says its state, with the same autocomplete', async () => {
    const { container, user } = screenIn('login');
    const password = () => container.querySelector<HTMLInputElement>('input[autocomplete="current-password"]');
    expect(password()?.type).toBe('password');
    expect(reveal().getAttribute('aria-pressed')).toBe('false');

    await user.click(reveal());
    expect(reveal().getAttribute('aria-pressed')).toBe('true');
    expect(password()?.type).toBe('text');
    expect(screen.getByLabelText(msg('transfers.authPasswordLabel'))).toBe(password());

    await user.click(reveal());
    expect(password()?.type).toBe('password');
  });

  it('keeps the new-password autocomplete on the sign-up form while shown, and is hidden again after a switch', async () => {
    const { user } = screenIn('register');
    await user.click(reveal());
    const password = field(msg('transfers.authPasswordLabel'));
    expect(password.type).toBe('text');
    expect(password.getAttribute('autocomplete')).toBe('new-password');

    await user.click(screen.getByRole('button', { name: msg('transfers.authSignInAction') }));
    expect(field(msg('transfers.authPasswordLabel')).type).toBe('password');
    expect(reveal().getAttribute('aria-pressed')).toBe('false');
  });

  it('comes after the password field in the Tab order', async () => {
    const { user } = screenIn('login');
    field(msg('transfers.authPasswordLabel')).focus();
    await user.tab();
    expect(document.activeElement).toBe(reveal());
  });
});

describe('AuthScreen: where a refusal sends focus', () => {
  it('moves focus from the pressed button to the field the refusal is about', async () => {
    account.Register.mockRejectedValue(new Error('username_taken'));
    const { user } = screenIn('register');
    await user.type(field(msg('common.email')), 'player@example.test');
    await user.type(field(msg('transfers.authUsernameLabel')), 'player_one');
    await user.type(field(msg('transfers.authDisplayNameLabel')), 'Player One');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-2');
    await user.click(screen.getByRole('button', { name: msg('transfers.authCreateAccount') }));

    const username = field(msg('transfers.authUsernameLabel'));
    await vi.waitFor(() => expect(username.getAttribute('aria-invalid')).toBe('true'));
    await vi.waitFor(() => expect(document.activeElement).toBe(username));
  });

  it('moves focus from the field Enter was pressed in to the field the refusal is about', async () => {
    account.Login.mockRejectedValue(new Error('invalid_username'));
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'x');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');

    const login = field(msg('transfers.authIdentifierLabel'));
    await vi.waitFor(() => expect(login.getAttribute('aria-invalid')).toBe('true'));
    await vi.waitFor(() => expect(document.activeElement).toBe(login));
  });

  it('says a refusal about a field the sign-in form does not have in the form, never nowhere', async () => {
    account.Login.mockRejectedValue(new Error('invalid_email'));
    const { container, user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player@');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1');
    const submit = screen.getByRole('button', { name: msg('transfers.authSignInAction') });
    await user.click(submit);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.accountErrorInvalidEmail'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(container.querySelectorAll('[aria-invalid="true"]')).toHaveLength(0);
    expect(document.activeElement).toBe(submit);
  });

  it('says a code the screen has no words for as its plain failure', async () => {
    account.Login.mockRejectedValue(new Error('account_muted'));
    const { user } = screenIn('login');
    await user.type(field(msg('transfers.authIdentifierLabel')), 'player_one');
    await user.type(field(msg('transfers.authPasswordLabel')), 'fake-pass-1{Enter}');

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('transfers.authGenericError'));
  });
});

describe('AuthScreen: a link that cannot open', () => {
  // The screen is outside the shell, which holds the toast stack: a toast raised here shows nowhere.
  it.each([
    ['tickets.forgotPassword', 'OpenForgotPassword', 'tickets.forgotOpenFailed'],
    ['tickets.cantSignIn', 'OpenSupportSite', 'tickets.siteOpenFailed'],
  ] as const)('%s says the failure on the screen and keeps focus on the link', async (link, method, text) => {
    account[method].mockRejectedValue(new Error('network_error'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = screenIn('login');
    const pressed = screen.getByRole('button', { name: msg(link) });
    await user.click(pressed);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg(text));
    expect(document.activeElement).toBe(pressed);
    error.mockRestore();
  });
});

describe('AuthScreen: the gate itself', () => {
  it('is the page, named by its heading, with one heading', () => {
    screenIn('login');
    const page = screen.getByRole('main', { name: msg('transfers.authLoginTitle') });
    expect(page.querySelectorAll('h1, h2, h3, h4, h5, h6')).toHaveLength(1);
    expect(page.contains(screen.getByRole('button', { name: msg('transfers.authGuestAction') }))).toBe(true);
  });

  it('is named by the out-of-reach heading when the server is out of reach', () => {
    authReason.set('network_error');
    authState.set('unavailable');
    renderDom(AuthScreen);
    expect(screen.getByRole('main', { name: msg('transfers.authOfflineTitle') })).toBeTruthy();
  });

  it("shows the shell's logo part, not a picture of its own", () => {
    const { container } = screenIn('login');
    const brand = container.querySelector('.brand');
    expect(brand?.querySelector('[data-logo-origin] img.logo-mark')).not.toBeNull();
    expect([...container.querySelectorAll('img')].filter((img) => !img.closest('[data-logo-origin]'))).toEqual([]);
  });
});

describe('the app\'s gate around the sign-in screen', () => {
  it('shows the boot splash by the store\'s answer, so a retry keeps the screen and its Retry mounted', async () => {
    const app = (await import('../../App.svelte?raw')).default;
    expect(app).toContain('{#if $bootSplash}');
    expect(app).not.toMatch(/\$authState === 'bootstrapping'/);
  });
});
