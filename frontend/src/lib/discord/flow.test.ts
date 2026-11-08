import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/account', () => ({ Service: {} }));

import { createDiscordFlow } from './flow';
import { AccountError } from '../services/account';
import type { DiscordApi, DiscordLink } from '../services/discord';

const unlinked: DiscordLink = { linked: false, discordId: '', discordUsername: '', linkedAt: '' };
const linked: DiscordLink = { linked: true, discordId: '1', discordUsername: 'fan', linkedAt: '2026-11-05T10:00:00Z' };

function fakeApi() {
  return {
    createLinkCode: vi.fn<DiscordApi['createLinkCode']>(),
    getLink: vi.fn<DiscordApi['getLink']>(),
    unlink: vi.fn<DiscordApi['unlink']>(),
  };
}

const T0 = Date.parse('2026-11-05T12:00:00Z');
const code = () => ({ code: 'ABCD2345', expiresAt: new Date(T0 + 10 * 60_000).toISOString() });

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(T0);
});
afterEach(() => vi.useRealTimers());

describe('discord link flow', () => {
  it('loads the unlinked and linked states', async () => {
    const api = fakeApi();
    api.getLink.mockResolvedValueOnce(unlinked).mockResolvedValueOnce(linked);
    const flow = createDiscordFlow(api);
    await flow.load();
    expect(get(flow.state).phase).toBe('unlinked');
    await flow.load();
    expect(get(flow.state).phase).toBe('linked');
    expect(get(flow.state).link?.discordUsername).toBe('fan');
    flow.dispose();
  });

  it('treats a missing backend feature as unavailable, not as an error', async () => {
    const api = fakeApi();
    api.getLink.mockRejectedValue(new AccountError('discord_unavailable'));
    const flow = createDiscordFlow(api);
    await flow.load();
    expect(get(flow.state).phase).toBe('unavailable');
    flow.dispose();
  });

  it('asks the caller to re-login when the session is gone', async () => {
    const api = fakeApi();
    api.getLink.mockRejectedValue(new AccountError('unauthenticated'));
    const lost = vi.fn();
    const flow = createDiscordFlow(api, { onUnauthenticated: lost });
    await flow.load();
    expect(lost).toHaveBeenCalledTimes(1);
    flow.dispose();
  });

  it('shows a retryable error for other load failures', async () => {
    const api = fakeApi();
    api.getLink.mockRejectedValue(new AccountError('network_error'));
    const flow = createDiscordFlow(api);
    await flow.load();
    expect(get(flow.state).phase).toBe('error');
    flow.dispose();
  });

  it('polls every 3 seconds while a code is active and stops on success', async () => {
    const api = fakeApi();
    api.createLinkCode.mockResolvedValue(code());
    api.getLink.mockResolvedValue(unlinked);
    const flow = createDiscordFlow(api);
    await flow.start();
    expect(get(flow.state)).toMatchObject({ phase: 'code', code: 'ABCD2345', remaining: 600 });

    await vi.advanceTimersByTimeAsync(9000);
    expect(api.getLink).toHaveBeenCalledTimes(3);
    expect(get(flow.state).remaining).toBe(591);

    api.getLink.mockResolvedValue(linked);
    await vi.advanceTimersByTimeAsync(3000);
    expect(get(flow.state).phase).toBe('linked');
    const calls = api.getLink.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30_000);
    expect(api.getLink.mock.calls.length).toBe(calls);
    flow.dispose();
  });

  it('keeps polling through a dropped poll', async () => {
    const api = fakeApi();
    api.createLinkCode.mockResolvedValue(code());
    api.getLink.mockRejectedValueOnce(new AccountError('network_error')).mockResolvedValue(linked);
    const flow = createDiscordFlow(api);
    await flow.start();
    await vi.advanceTimersByTimeAsync(6000);
    expect(get(flow.state).phase).toBe('linked');
    flow.dispose();
  });

  it('expires the code and stops polling', async () => {
    const api = fakeApi();
    api.createLinkCode.mockResolvedValue({ code: 'ABCD2345', expiresAt: new Date(T0 + 5000).toISOString() });
    api.getLink.mockResolvedValue(unlinked);
    const flow = createDiscordFlow(api);
    await flow.start();
    await vi.advanceTimersByTimeAsync(6000);
    expect(get(flow.state)).toMatchObject({ phase: 'unlinked', expired: true, code: '' });
    const calls = api.getLink.mock.calls.length;
    await vi.advanceTimersByTimeAsync(30_000);
    expect(api.getLink.mock.calls.length).toBe(calls);
    flow.dispose();
  });

  it('stops polling when the page is left', async () => {
    const api = fakeApi();
    api.createLinkCode.mockResolvedValue(code());
    api.getLink.mockResolvedValue(unlinked);
    const flow = createDiscordFlow(api);
    await flow.start();
    flow.dispose();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(api.getLink).not.toHaveBeenCalled();
  });

  it('surfaces rate limiting and keeps the unlinked state', async () => {
    const api = fakeApi();
    api.getLink.mockResolvedValue(unlinked);
    api.createLinkCode.mockRejectedValue(new AccountError('rate_limited'));
    const flow = createDiscordFlow(api);
    await flow.load();
    await flow.start();
    expect(get(flow.state)).toMatchObject({ phase: 'unlinked', errorCode: 'rate_limited', busy: false });
    flow.dispose();
  });

  it('marks the backend unavailable when issuing a code gets a 404', async () => {
    const api = fakeApi();
    api.createLinkCode.mockRejectedValue(new AccountError('discord_unavailable'));
    const flow = createDiscordFlow(api);
    await flow.start();
    expect(get(flow.state).phase).toBe('unavailable');
    flow.dispose();
  });

  it('unlinks and returns to the unlinked state; a failure keeps the link', async () => {
    const api = fakeApi();
    api.getLink.mockResolvedValue(linked);
    const flow = createDiscordFlow(api);
    await flow.load();
    api.unlink.mockRejectedValueOnce(new AccountError('network_error'));
    await flow.unlink();
    expect(get(flow.state)).toMatchObject({ phase: 'linked', errorCode: 'network_error', busy: false });
    api.unlink.mockResolvedValueOnce(undefined);
    await flow.unlink();
    expect(get(flow.state)).toMatchObject({ phase: 'unlinked', link: null });
    flow.dispose();
  });
});
