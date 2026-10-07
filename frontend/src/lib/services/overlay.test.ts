import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  Status: vi.fn(),
  Hide: vi.fn(),
  OpenBrowser: vi.fn(),
  PlaceBrowser: vi.fn(),
};

const events = {
  On: vi.fn(),
  Emit: vi.fn(),
};

vi.mock('@wailsio/runtime', () => ({ Events: events }));
vi.mock('../../../bindings/typhon/internal/overlay', () => ({ Service: bindings }));

async function loadOverlay(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./overlay');
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('toOverlayStatus', () => {
  it('reads a complete status as it is', async () => {
    const overlay = await loadOverlay(true);

    expect(
      overlay.toOverlayStatus({ supported: true, enabled: true, hotkey: 'Shift+F1', error: 'taken' }),
    ).toEqual({ supported: true, enabled: true, hotkey: 'Shift+F1', error: 'taken' });
  });

  it('turns a missing or malformed status into an unsupported, disabled overlay with the default hotkey', async () => {
    const overlay = await loadOverlay(true);
    const empty = {
      supported: false,
      enabled: false,
      hotkey: overlay.DEFAULT_OVERLAY_HOTKEY,
      error: '',
    };

    expect(overlay.toOverlayStatus(null)).toEqual(empty);
    expect(overlay.toOverlayStatus(undefined)).toEqual(empty);
    expect(overlay.toOverlayStatus({ supported: 'yes', enabled: 1, hotkey: '' })).toEqual(empty);
  });

  it('does not take a truthy non-boolean for a yes', async () => {
    const overlay = await loadOverlay(true);

    expect(overlay.toOverlayStatus({ supported: 'true', enabled: 1 }).supported).toBe(false);
    expect(overlay.toOverlayStatus({ supported: 'true', enabled: 1 }).enabled).toBe(false);
  });
});

describe('overlay bridge', () => {
  it('answers a browser preview without calling the backend', async () => {
    const overlay = await loadOverlay(false);

    await expect(overlay.overlayStatus()).resolves.toMatchObject({ supported: false, enabled: false });
    expect(overlay.onOverlayStatus(() => {})).toBeTypeOf('function');
    expect(bindings.Status).not.toHaveBeenCalled();
    expect(events.On).not.toHaveBeenCalled();
  });

  it('maps the status the backend reports', async () => {
    bindings.Status.mockResolvedValue({ supported: true, enabled: true, hotkey: 'Alt+`' });
    const overlay = await loadOverlay(true);

    await expect(overlay.overlayStatus()).resolves.toMatchObject({ supported: true, enabled: true });
  });

  it('maps every pushed status through the same rules and returns the unsubscribe', async () => {
    const off = vi.fn();
    let push: (event: { data: unknown }) => void = () => {};
    events.On.mockImplementation((name: string, cb: (event: { data: unknown }) => void) => {
      expect(name).toBe('overlay:status');
      push = cb;
      return off;
    });
    const overlay = await loadOverlay(true);
    const seen: unknown[] = [];

    const stop = overlay.onOverlayStatus((status) => seen.push(status));
    push({ data: { supported: true, enabled: false, hotkey: 'Shift+F2' } });
    push({ data: null });

    expect(seen).toEqual([
      { supported: true, enabled: false, hotkey: 'Shift+F2', error: '' },
      { supported: false, enabled: false, hotkey: overlay.DEFAULT_OVERLAY_HOTKEY, error: '' },
    ]);
    expect(stop).toBe(off);
  });

  it('subscribes to the named overlay event and calls the handler without its payload', async () => {
    let push: (event: { data: unknown }) => void = () => {};
    events.On.mockImplementation((_name: string, cb: (event: { data: unknown }) => void) => {
      push = cb;
      return vi.fn();
    });
    const overlay = await loadOverlay(true);
    const handler = vi.fn();

    overlay.onOverlayEvent('overlay:hidden', handler);
    push({ data: { secret: 1 } });

    expect(events.On).toHaveBeenCalledWith('overlay:hidden', expect.any(Function));
    expect(handler).toHaveBeenCalledWith();
  });

  it('hides through the event when it can', async () => {
    events.Emit.mockResolvedValue(undefined);
    const overlay = await loadOverlay(true);

    await overlay.hideOverlay();

    expect(events.Emit).toHaveBeenCalledWith('overlay:hide');
    expect(bindings.Hide).not.toHaveBeenCalled();
  });

  it('falls back to the direct call when the event cannot be sent', async () => {
    events.Emit.mockRejectedValue(new Error('no listeners'));
    bindings.Hide.mockResolvedValue(undefined);
    const overlay = await loadOverlay(true);

    await overlay.hideOverlay();

    expect(bindings.Hide).toHaveBeenCalledTimes(1);
  });

  it('lets a failure of the fallback reach the caller', async () => {
    events.Emit.mockRejectedValue(new Error('no listeners'));
    bindings.Hide.mockRejectedValue(new Error('overlay window is gone'));
    const overlay = await loadOverlay(true);

    await expect(overlay.hideOverlay()).rejects.toThrow('overlay window is gone');
  });
});

describe('overlay window detection and geometry', () => {
  it('recognises the overlay window by its query flag', async () => {
    const overlay = await loadOverlay(true);

    vi.stubGlobal('window', { location: { search: '?overlay' } });
    expect(overlay.isOverlayWindow()).toBe(true);

    vi.stubGlobal('window', { location: { search: '?server' } });
    expect(overlay.isOverlayWindow()).toBe(false);
  });

  it('scales the browser area to device pixels and never loses a pixel to rounding', async () => {
    const overlay = await loadOverlay(true);
    vi.stubGlobal('window', { devicePixelRatio: 1.5 });
    const element = { getBoundingClientRect: () => ({ left: 10.4, top: 20.6, right: 110.2, bottom: 80.9 }) };

    const area = overlay.browserArea(element as unknown as HTMLElement);

    expect(area).toEqual({ x: 16, y: 31, width: 149, height: 90 });
  });

  it('uses a scale of one when the ratio is missing', async () => {
    const overlay = await loadOverlay(true);
    vi.stubGlobal('window', {});
    const element = { getBoundingClientRect: () => ({ left: 5, top: 6, right: 25, bottom: 46 }) };

    expect(overlay.browserArea(element as unknown as HTMLElement)).toEqual({ x: 5, y: 6, width: 20, height: 40 });
  });
});
