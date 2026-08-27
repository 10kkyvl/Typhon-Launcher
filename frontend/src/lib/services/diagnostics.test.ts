import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  ReportClientError: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/diagnostics', () => ({ Service: bindings }));

type FakeEvent = { type: string } & Record<string, unknown>;

class FakeWindow {
  private listeners = new Map<string, Array<(event: FakeEvent) => void>>();

  addEventListener(type: string, handler: (event: FakeEvent) => void) {
    const list = this.listeners.get(type) ?? [];
    list.push(handler);
    this.listeners.set(type, list);
  }

  removeEventListener() {}

  dispatchEvent(event: FakeEvent) {
    for (const handler of this.listeners.get(event.type) ?? []) handler(event);
    return true;
  }
}

function errorEvent(error: unknown, message = 'boom'): FakeEvent {
  return { type: 'error', error, message };
}

function rejectionEvent(reason: unknown): FakeEvent {
  return { type: 'unhandledrejection', reason };
}

let win: FakeWindow;

beforeEach(() => {
  vi.clearAllMocks();
  vi.resetModules();
  win = new FakeWindow();
  vi.stubGlobal('window', win);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('installDiagnostics inside Wails', () => {
  it('reports a window error with sanitized fields', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValueOnce(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const err = new Error('load failed for C:\\Users\\Egor\\AppData\\Local\\Typhon\\game.exe');
    err.stack = 'Error: load failed\n    at start (C:\\Users\\Egor\\AppData\\Local\\Typhon\\game.exe:10:5)';
    win.dispatchEvent(errorEvent(err, err.message));

    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);
    const [component, operation, message, stack, fatal] = bindings.ReportClientError.mock.calls[0];
    expect(component).toBe('frontend');
    expect(operation).toBe('window.onerror');
    expect(message).not.toContain('Egor');
    expect(stack).not.toContain('Egor');
    expect(stack).toContain('at start');
    expect(fatal).toBe(true);
  });

  it('reports an unhandled promise rejection', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValueOnce(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const reason = new Error('rejected');
    win.dispatchEvent(rejectionEvent(reason));

    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);
    const [component, operation, message, , fatal] = bindings.ReportClientError.mock.calls[0];
    expect(component).toBe('frontend');
    expect(operation).toBe('window.unhandledrejection');
    expect(message).toBe('rejected');
    expect(fatal).toBe(false);
  });

  it('scrubs Windows paths, Unix paths, macOS paths, magnet URIs, infohashes and bearer tokens', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValueOnce(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const secrets = [
      'C:\\Users\\Egor\\AppData\\Local\\Typhon\\typhon.log',
      '/home/egor/.config/typhon/typhon.log',
      '/Users/egor/Library/Application Support/Typhon',
      'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=Game',
      '0123456789abcdef0123456789abcdef01234567',
      'Authorization: Bearer abcDEF123.token-value_1',
      'https://api.typhon.app/v1/download?token=secret&user=egor',
    ].join(' | ');

    const err = new Error(secrets);
    err.stack = `Error: ${secrets}`;
    win.dispatchEvent(errorEvent(err, err.message));

    const [, , message, stack] = bindings.ReportClientError.mock.calls[0];
    for (const value of [message, stack]) {
      expect(value).not.toContain('Egor');
      expect(value).not.toContain('egor');
      expect(value).not.toContain('btih:0123456789abcdef0123456789abcdef01234567');
      expect(value).not.toContain('0123456789abcdef0123456789abcdef01234567');
      expect(value).not.toContain('abcDEF123.token-value_1');
      expect(value).not.toContain('?token=secret&user=egor');
      expect(value).toContain('<path>');
      expect(value).not.toContain('C:\\Users');
      expect(value).not.toContain('/home/');
      expect(value).toContain('<magnet>');
      expect(value).toContain('<hash>');
      expect(value).toContain('Bearer <token>');
      expect(value).toContain('https://api.typhon.app/v1/download');
    }
  });

  it('truncates message and stack at the caps shared with the backend', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValueOnce(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const err = new Error('m'.repeat(5000));
    err.stack = 's'.repeat(20000);
    win.dispatchEvent(errorEvent(err, err.message));

    const [, , message, stack] = bindings.ReportClientError.mock.calls[0];
    expect((message as string).length).toBe(2000);
    expect((stack as string).length).toBe(8000);
  });

  it('dedupes and throttles repeated identical errors', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValue(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const err = new Error('render loop failed');
    for (let i = 0; i < 50; i++) {
      win.dispatchEvent(errorEvent(err, err.message));
    }

    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);
  });

  it('does not double-report when installed twice', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockResolvedValueOnce(undefined);
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();
    installDiagnostics();

    win.dispatchEvent(errorEvent(new Error('single report'), 'single report'));

    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);
  });

  it('does not propagate or loop when the binding call itself throws', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    bindings.ReportClientError.mockImplementationOnce(() => {
      throw new Error('backend unreachable');
    });
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    const err = new Error('breaks the binding');
    expect(() => win.dispatchEvent(errorEvent(err, err.message))).not.toThrow();
    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);

    expect(() => win.dispatchEvent(errorEvent(err, err.message))).not.toThrow();
    expect(bindings.ReportClientError).toHaveBeenCalledTimes(1);
  });
});

describe('installDiagnostics outside Wails', () => {
  it('is a no-op', async () => {
    vi.doMock('./backend', () => ({ inWails: false }));
    const { installDiagnostics } = await import('./diagnostics');
    installDiagnostics();

    win.dispatchEvent(errorEvent(new Error('should be ignored'), 'should be ignored'));
    win.dispatchEvent(rejectionEvent(new Error('should be ignored too')));

    expect(bindings.ReportClientError).not.toHaveBeenCalled();
  });
});
