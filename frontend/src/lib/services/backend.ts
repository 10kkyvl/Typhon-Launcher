type BridgeWindow = Window & {
  chrome?: { webview?: { postMessage?: unknown } };
  webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
  wails?: { invoke?: unknown };
};

const w = window as BridgeWindow;

export const inWails =
  Boolean(
    w.chrome?.webview?.postMessage || w.webkit?.messageHandlers?.external?.postMessage || w.wails?.invoke,
  ) || new URLSearchParams(window.location.search).has('server');
