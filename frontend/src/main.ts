import { mount } from 'svelte';
import '@fontsource-variable/inter-tight';
import './styles/tokens.css';
import './styles/global.css';
import App from './App.svelte';
import { installDiagnostics } from './lib/services/diagnostics';
import { mountThemeGuard } from './lib/components/ThemeGuard';
import { initTheme, resetAppearance } from './lib/stores/theme';

installDiagnostics();

mount(App, { target: document.getElementById('app')! });

initTheme();
mountThemeGuard();

window.addEventListener('keydown', (event) => {
  if (event.ctrlKey && event.shiftKey && event.altKey && event.code === 'KeyT') {
    event.preventDefault();
    resetAppearance();
  }
});
