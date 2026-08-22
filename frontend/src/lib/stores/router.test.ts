import { beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';

async function loadRouter() {
  const mod = await import('./router');
  mod.resetHistory();
  return mod;
}

beforeEach(async () => {
  const { resetHistory } = await import('./router');
  resetHistory();
});

describe('router', () => {
  it('starts on the library route', async () => {
    const { route } = await loadRouter();
    expect(get(route).name).toBe('library');
  });

  it('navigates to the profile route with no params', async () => {
    const { navigate, route } = await loadRouter();
    navigate('profile');
    expect(get(route)).toEqual({ name: 'profile', params: {} });
  });

  it('keeps profile and settings as separate destinations', async () => {
    const { navigate, route, canGoBack, goBack } = await loadRouter();

    navigate('settings');
    expect(get(route).name).toBe('settings');

    navigate('profile');
    expect(get(route).name).toBe('profile');
    expect(get(canGoBack)).toBe(true);

    goBack();
    expect(get(route).name).toBe('settings');
  });

  it('resetHistory drops the stack back to a single library entry', async () => {
    const { navigate, resetHistory, route, canGoBack, canGoForward } = await loadRouter();

    navigate('settings');
    navigate('profile');
    resetHistory();

    expect(get(route)).toEqual({ name: 'library', params: {} });
    expect(get(canGoBack)).toBe(false);
    expect(get(canGoForward)).toBe(false);
  });
});
