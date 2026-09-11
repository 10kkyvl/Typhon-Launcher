import { toast } from './lib/stores/toasts';
import { msg } from './lib/i18n';
import { initSettings } from './lib/stores/settings';
import { mount } from 'svelte';
import '@fontsource-variable/inter-tight';
import './styles/tokens.css';
import './styles/global.css';
import App from './App.svelte';
import { installDiagnostics } from './lib/services/diagnostics';
import { mountThemeGuard } from './lib/components/ThemeGuard';
import { initTheme, resetAppearance } from './lib/stores/theme';

installDiagnostics();

async function start() {
  try {
    await initSettings();
  } catch (err) {
    console.error('load settings', err);
    toast(msg('state.settingsNotLoaded'), 'danger');
  }
  await initTheme();
  mount(App, { target: document.getElementById('app')! });
}
void start();
mountThemeGuard();

window.addEventListener('keydown', (event) => {
  if (event.ctrlKey && event.shiftKey && event.altKey && event.code === 'KeyT') {
    event.preventDefault();
    resetAppearance();
  }
});
