import { toast } from './lib/stores/toasts';
import { msg } from './lib/i18n';
import { initSettings } from './lib/stores/settings';
import { mount } from 'svelte';
import '@fontsource-variable/inter-tight';
import './styles/tokens.css';
import './styles/global.css';
import { installDiagnostics } from './lib/services/diagnostics';
import { isOverlayWindow } from './lib/services/overlay';
import { mountThemeGuard } from './lib/components/ThemeGuard';
import { initTheme, resetAppearance } from './lib/stores/theme';

const overlay = isOverlayWindow();

installDiagnostics();

async function start() {
  let settingsFailed = false;
  try {
    await initSettings();
  } catch (err) {
    console.error('load settings', err);
    settingsFailed = true;
    if (!overlay) toast(msg('state.settingsNotLoaded'), 'danger');
  }
  await initTheme();
  const target = document.getElementById('app')!;
  try {
    if (overlay) {
      const { default: Overlay } = await import('./overlay/Overlay.svelte');
      mount(Overlay, { target, props: { settingsFailed } });
      return;
    }
    const { default: App } = await import('./App.svelte');
    mount(App, { target });
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
