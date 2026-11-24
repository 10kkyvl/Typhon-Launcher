import { toast } from './lib/stores/toasts';
import { msg } from './lib/i18n';
import { initSettings } from './lib/stores/settings';
import { mount } from 'svelte';
import '@fontsource-variable/onest';
import '@fontsource-variable/jetbrains-mono';
// No unicode-range in these files: the face declared last is tried first, so latin stays last.
import '@fontsource/unbounded/cyrillic-500.css';
import '@fontsource/unbounded/latin-500.css';
import './styles/tokens.css';
import './styles/shell.css';
import './styles/global.css';
import { installDiagnostics } from './lib/services/diagnostics';
import { isOverlayWindow } from './lib/services/overlay';
import { mountThemeGuard } from './lib/components/ThemeGuard';
import { initTheme, resetAppearance } from './lib/stores/theme';
import { initPlatform } from './lib/stores/window';
import { loadHandoff } from './lib/stores/handoff';
import { updateShow } from './lib/stores/updateShow';

const overlay = isOverlayWindow();

installDiagnostics();

function bootBackground(): HTMLElement | null {
  return document.getElementById('boot-bg');
}

// The boot colour stands in until the stylesheets and the theme are in. A successor of a seamless
// restart opens on the old window's colour and keeps it until the show it took over is done.
function dropBootBackground(handedOver: boolean) {
  if (!handedOver) {
    bootBackground()?.remove();
    return;
  }
  let off = () => {};
  off = updateShow.subscribe((show) => {
    if (show) return;
    bootBackground()?.remove();
    queueMicrotask(() => off());
  });
}

async function start() {
  const handoffRead = overlay ? Promise.resolve(null) : loadHandoff();
  const platformReady = overlay ? Promise.resolve() : initPlatform();
  const handoff = await handoffRead;
  const boot = bootBackground();
  if (handoff && boot && /^#[0-9a-f]{6}$/i.test(handoff.background)) boot.textContent = `html, body { background: ${handoff.background}; }`;
  let settingsFailed = false;
  try {
    await initSettings();
  } catch (err) {
    console.error('load settings', err);
    settingsFailed = true;
    if (!overlay) toast(msg('state.settingsNotLoaded'), 'danger');
  }
  await initTheme();
  await platformReady;
  const target = document.getElementById('app')!;
  try {
    if (overlay) {
      const { default: Overlay } = await import('./overlay/Overlay.svelte');
      mount(Overlay, { target, props: { settingsFailed } });
      return;
    }
    const { default: App } = await import('./App.svelte');
    mount(App, { target });
    dropBootBackground(handoff !== null);
  } catch (err) {
    console.error('load interface', err);
    target.textContent = msg('state.uiLoadFailed');
  }
}

if (overlay) document.documentElement.classList.add('overlay-window');
void start();

if (!overlay) {
  mountThemeGuard();

  window.addEventListener('keydown', (event) => {
    if (event.ctrlKey && event.shiftKey && event.altKey && event.code === 'KeyT') {
      event.preventDefault();
      resetAppearance();
    }
  });
}
