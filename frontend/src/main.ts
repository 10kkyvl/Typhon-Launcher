import { mount } from 'svelte';
import '@fontsource-variable/inter-tight';
import './styles/tokens.css';
import './styles/global.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
