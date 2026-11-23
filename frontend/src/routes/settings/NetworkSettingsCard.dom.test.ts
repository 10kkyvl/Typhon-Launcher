import { screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
// Every request of the card is a mock: no proxy is ever reached.
vi.mock('../../lib/services/network', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/network')>()),
  hasProxyPassword: vi.fn(),
  listNetworkInterfaces: vi.fn(),
  setProxyPassword: vi.fn(),
  testProxy: vi.fn(),
}));

import { msg, type MessageKey } from '../../lib/i18n';
import { networkErrorText } from '../../lib/network/networkText';
import { hasProxyPassword, listNetworkInterfaces, setProxyPassword, testProxy } from '../../lib/services/network';
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { networkState } from '../../lib/stores/network';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import NetworkSettingsCard from './NetworkSettingsCard.svelte';

const DIRECT = { networkMode: 'direct', networkInterface: '', proxyType: 'socks5', proxyHost: '', proxyPort: 0, proxyUsername: '' };
const PROXY = { networkMode: 'proxy', networkInterface: '', proxyType: 'http', proxyHost: 'proxy.lan', proxyPort: 3128, proxyUsername: 'me' };
const ETH = { name: 'eth0', description: '', addresses: ['10.0.0.2'], up: true, vpnLike: false };

const toastTexts = () => get(toasts).map((item) => item.message);
const field = (key: MessageKey) => screen.getByLabelText(msg(key), { selector: 'input' }) as HTMLInputElement;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  settings.set({ ...DIRECT } as unknown as SettingsData);
  vi.mocked(hasProxyPassword).mockResolvedValue(false);
  vi.mocked(listNetworkInterfaces).mockResolvedValue([ETH]);
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
});

afterEach(() => {
  document.body.replaceChildren();
  settings.set(null);
  networkState.set(null);
  toasts.set([]);
  vi.restoreAllMocks();
  for (const mock of [hasProxyPassword, listNetworkInterfaces, setProxyPassword, testProxy, saveSettingsPatch]) vi.mocked(mock).mockReset();
});

describe('NetworkSettingsCard: the mode', () => {
  it('is a radio group named by its label', () => {
    renderDom(NetworkSettingsCard, {});
    const group = screen.getByRole('radiogroup', { name: msg('settings.networkModeLabel') });
    expect(description(group)).toBe('');
    expect(screen.getByRole('radio', { name: msg('settings.networkModeDirect') }).getAttribute('aria-checked')).toBe('true');
  });

  it('adds the interface row after the group when the interface mode is chosen, focus kept on the choice', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    const group = screen.getByRole('radiogroup', { name: msg('settings.networkModeLabel') });
    screen.getByRole('radio', { name: msg('settings.networkModeDirect') }).focus();

    await user.keyboard('{ArrowRight}');

    const chosen = screen.getByRole('radio', { name: msg('settings.networkModeInterface') });
    expect(document.activeElement).toBe(chosen);
    const list = await screen.findByRole('combobox', { name: msg('settings.networkInterfaceLabel') });
    expect(group.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(listNetworkInterfaces).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: msg('settings.networkInterfaceRefresh') })).toBeTruthy();
  });

  it('applies a chosen interface', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.click(screen.getByRole('radio', { name: msg('settings.networkModeInterface') }));
    await user.click(await screen.findByRole('combobox', { name: msg('settings.networkInterfaceLabel') }));
    await user.click(await screen.findByRole('option', { name: 'eth0 · 10.0.0.2' }));

    await user.click(screen.getByRole('button', { name: msg('settings.networkApply') }));

    await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ networkMode: 'interface', networkInterface: 'eth0' }));
    await waitFor(() => expect(toastTexts()).toContain(msg('settings.networkAppliedToast')));
  });

  it('says the interfaces could not be listed on a danger notice', async () => {
    vi.mocked(listNetworkInterfaces).mockRejectedValue(new Error('down'));
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.click(screen.getByRole('radio', { name: msg('settings.networkModeInterface') }));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(networkErrorText(new Error('down')));
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });

  it('adds the proxy rows after the group when the proxy is chosen', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    const group = screen.getByRole('radiogroup', { name: msg('settings.networkModeLabel') });
    await user.click(screen.getByRole('radio', { name: msg('settings.networkModeProxy') }));
    const type = screen.getByRole('combobox', { name: msg('settings.networkProxyTypeLabel') });
    expect(group.compareDocumentPosition(type) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(field('settings.networkProxyHostLabel')).toBeTruthy();
  });
});

describe('NetworkSettingsCard: the proxy fields', () => {
  beforeEach(() => {
    settings.set({ ...PROXY } as unknown as SettingsData);
  });

  it('names each field by its label', () => {
    renderDom(NetworkSettingsCard, {});
    expect(field('settings.networkProxyHostLabel').value).toBe('proxy.lan');
    expect(field('settings.networkProxyPortLabel').value).toBe('3128');
    expect(field('settings.networkProxyUsernameLabel').value).toBe('me');
    expect(field('settings.networkProxyPasswordLabel').type).toBe('password');
    expect(screen.getByRole('combobox', { name: msg('settings.networkProxyTypeLabel') }).textContent).toContain('HTTP');
  });

  it('describes no field by its text yet', () => {
    renderDom(NetworkSettingsCard, {});
    expect(description(field('settings.networkProxyHostLabel'))).toBe('');
    expect(description(field('settings.networkProxyUsernameLabel'))).toBe('');
    expect(description(field('settings.networkProxyPasswordLabel'))).toBe('');
  });

  it('applies the proxy, the password first', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.clear(field('settings.networkProxyHostLabel'));
    await user.type(field('settings.networkProxyHostLabel'), 'other.lan');
    await user.type(field('settings.networkProxyPasswordLabel'), 'secret');

    await user.click(screen.getByRole('button', { name: msg('settings.networkApply') }));

    await waitFor(() =>
      expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ networkMode: 'proxy', proxyType: 'http', proxyHost: 'other.lan', proxyPort: 3128, proxyUsername: 'me' }),
    );
    expect(setProxyPassword).toHaveBeenCalledExactlyOnceWith('me', 'secret');
    expect(vi.mocked(setProxyPassword).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(saveSettingsPatch).mock.invocationCallOrder[0]);
    await waitFor(() => expect(field('settings.networkProxyPasswordLabel').value).toBe(''));
    expect(screen.getByRole('button', { name: msg('settings.networkProxyPasswordDelete') })).toBeTruthy();
  });

  it('says a password that was not stored, and saves no settings', async () => {
    vi.mocked(setProxyPassword).mockRejectedValue(new Error('keychain'));
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.type(field('settings.networkProxyPasswordLabel'), 'secret');

    await user.click(screen.getByRole('button', { name: msg('settings.networkApply') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(networkErrorText(new Error('keychain')));
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('deletes a stored password', async () => {
    vi.mocked(hasProxyPassword).mockResolvedValue(true);
    vi.mocked(setProxyPassword).mockResolvedValue();
    const { user } = renderDom(NetworkSettingsCard, {});
    const remove = await screen.findByRole('button', { name: msg('settings.networkProxyPasswordDelete') });
    expect(field('settings.networkProxyPasswordLabel').placeholder).toBe(msg('settings.networkProxyPasswordSaved'));

    await user.click(remove);

    expect(setProxyPassword).toHaveBeenCalledExactlyOnceWith('me', '');
    await waitFor(() => expect(screen.queryByRole('button', { name: msg('settings.networkProxyPasswordDelete') })).toBeNull());
    expect(toastTexts()).toContain(msg('settings.networkProxyPasswordDeletedToast'));
  });

  it('says a password that could not be deleted, the button kept', async () => {
    vi.mocked(hasProxyPassword).mockResolvedValue(true);
    vi.mocked(setProxyPassword).mockRejectedValue(new Error('keychain'));
    const { user } = renderDom(NetworkSettingsCard, {});

    await user.click(await screen.findByRole('button', { name: msg('settings.networkProxyPasswordDelete') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(networkErrorText(new Error('keychain')));
    expect(screen.getByRole('button', { name: msg('settings.networkProxyPasswordDelete') })).toBeTruthy();
  });
});

describe('NetworkSettingsCard: the proxy check', () => {
  beforeEach(() => {
    settings.set({ ...PROXY } as unknown as SettingsData);
  });

  it('checks the saved proxy', async () => {
    vi.mocked(testProxy).mockResolvedValue();
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.click(screen.getByRole('button', { name: msg('settings.networkProxyTest') }));
    expect(testProxy).toHaveBeenCalledTimes(1);
    expect(await screen.findByText(msg('settings.networkProxyTestOk'))).toBeTruthy();
  });

  it('says a failed check on a danger notice', async () => {
    vi.mocked(testProxy).mockRejectedValue(new Error('refused'));
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.click(screen.getByRole('button', { name: msg('settings.networkProxyTest') }));
    const said = await screen.findByText(networkErrorText(new Error('refused')));
    expect(said.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });

  it('checks nothing while an edit is not applied, and says to apply first', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.type(field('settings.networkProxyHostLabel'), 'x');
    const check = screen.getByRole('button', { name: msg('settings.networkProxyTest') }) as HTMLButtonElement;
    expect(screen.getByText(msg('settings.networkProxyTestNeedsApply'))).toBeTruthy();

    await user.click(check);

    expect(testProxy).not.toHaveBeenCalled();
  });
});

describe('NetworkSettingsCard: the status', () => {
  it('shows the state of the network with its address', () => {
    networkState.set({ mode: 'direct', state: 'ok', code: '', reason: '', address: '10.0.0.2', warning: '' });
    renderDom(NetworkSettingsCard, {});
    expect(screen.getByText(msg('settings.networkStatusLabel'))).toBeTruthy();
    expect(screen.getByText('10.0.0.2')).toBeTruthy();
    expect(screen.getByText(msg('settings.networkStatusOk'))).toBeTruthy();
  });
});
