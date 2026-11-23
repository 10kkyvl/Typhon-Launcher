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
  it('is a radio group named by its label and described by its hint', () => {
    renderDom(NetworkSettingsCard, {});
    const group = screen.getByRole('radiogroup', { name: msg('settings.networkModeLabel') });
    expect(description(group)).toBe(msg('settings.networkModeSub'));
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
    expect(description(list)).toBe(msg('settings.networkInterfaceSub'));
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

  it('names each field once, by a label element, and describes it by its hint', () => {
    renderDom(NetworkSettingsCard, {});
    const hints: Array<[MessageKey, MessageKey | null]> = [
      ['settings.networkProxyHostLabel', 'settings.networkProxyHostSub'],
      ['settings.networkProxyPortLabel', null],
      ['settings.networkProxyUsernameLabel', 'settings.networkProxyUsernameSub'],
      ['settings.networkProxyPasswordLabel', 'settings.networkProxyPasswordSub'],
    ];
    for (const [label, hint] of hints) {
      const input = field(label);
      expect(input.hasAttribute('aria-label')).toBe(false);
      expect(input.labels?.[0]?.textContent).toBe(msg(label));
      expect(description(input)).toBe(hint ? msg(hint) : '');
    }
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

  it('deletes a stored password, busy while it runs, then hands focus to the password field', async () => {
    vi.mocked(hasProxyPassword).mockResolvedValue(true);
    let answer: () => void = () => {};
    vi.mocked(setProxyPassword).mockImplementation(() => new Promise<void>((resolve) => (answer = resolve)));
    const { user } = renderDom(NetworkSettingsCard, {});
    const remove = await screen.findByRole('button', { name: msg('settings.networkProxyPasswordDelete') });
    expect(field('settings.networkProxyPasswordLabel').placeholder).toBe(msg('settings.networkProxyPasswordSaved'));
    expect(field('settings.networkProxyPasswordLabel').compareDocumentPosition(remove) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    remove.focus();
    await user.keyboard('{Enter}');

    expect(setProxyPassword).toHaveBeenCalledExactlyOnceWith('me', '');
    expect(remove.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(remove);
    answer();
    await waitFor(() => expect(screen.queryByRole('button', { name: msg('settings.networkProxyPasswordDelete') })).toBeNull());
    expect(toastTexts()).toContain(msg('settings.networkProxyPasswordDeletedToast'));
    await waitFor(() => expect(document.activeElement).toBe(field('settings.networkProxyPasswordLabel')));
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

  it('checks the saved proxy, busy and keeping focus while it runs', async () => {
    let answer: () => void = () => {};
    vi.mocked(testProxy).mockImplementation(() => new Promise<void>((resolve) => (answer = resolve)));
    const { user } = renderDom(NetworkSettingsCard, {});
    const check = screen.getByRole('button', { name: msg('settings.networkProxyTest') });
    check.focus();
    await user.keyboard('{Enter}');
    expect(testProxy).toHaveBeenCalledTimes(1);
    expect(check.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(check);
    answer();
    expect(await screen.findByText(msg('settings.networkProxyTestOk'))).toBeTruthy();
    expect(document.activeElement).toBe(check);
  });

  it('says a failed check on a danger notice', async () => {
    vi.mocked(testProxy).mockRejectedValue(new Error('refused'));
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.click(screen.getByRole('button', { name: msg('settings.networkProxyTest') }));
    const said = await screen.findByText(networkErrorText(new Error('refused')));
    expect(said.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });

  it('checks nothing while an edit is not applied, and says why through its reason, still reachable', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.type(field('settings.networkProxyHostLabel'), 'x');
    const check = screen.getByRole('button', { name: msg('settings.networkProxyTest') }) as HTMLButtonElement;
    expect(check.getAttribute('aria-disabled')).toBe('true');
    expect(check.disabled).toBe(false);
    expect(description(check)).toBe(msg('settings.networkProxyTestNeedsApply'));

    await user.click(check);

    expect(testProxy).not.toHaveBeenCalled();
  });
});

describe('NetworkSettingsCard: Apply', () => {
  beforeEach(() => {
    settings.set({ ...PROXY } as unknown as SettingsData);
  });

  it('says why it is off while nothing changed', () => {
    renderDom(NetworkSettingsCard, {});
    const apply = screen.getByRole('button', { name: msg('settings.networkApply') });
    expect(apply.getAttribute('aria-disabled')).toBe('true');
    expect(description(apply)).toBe(msg('settings.networkApplyNothing'));
  });

  it('keeps focus through a good Apply, busy while it runs, then off with its reason', async () => {
    let answer: (value: SettingsData) => void = () => {};
    vi.mocked(saveSettingsPatch).mockImplementation((patch) => new Promise<SettingsData>((resolve) => (answer = () => resolve({ ...get(settings), ...patch } as SettingsData))));
    const { user } = renderDom(NetworkSettingsCard, {});
    await user.type(field('settings.networkProxyHostLabel'), 'x');
    const apply = screen.getByRole('button', { name: msg('settings.networkApply') });
    expect(apply.hasAttribute('aria-disabled')).toBe(false);

    apply.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledTimes(1));
    expect(apply.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(apply);

    answer({} as SettingsData);
    await waitFor(() => expect(apply.getAttribute('aria-disabled')).toBe('true'));
    expect(document.activeElement).toBe(apply);
    expect(description(apply)).toBe(msg('settings.networkApplyNothing'));
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
