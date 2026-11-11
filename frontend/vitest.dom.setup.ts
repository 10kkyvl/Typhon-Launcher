import '@testing-library/svelte/vitest';
import { afterEach, vi } from 'vitest';
import { locale } from './src/lib/i18n';

locale.set('ru');

afterEach(() => {
  vi.useRealTimers();
  locale.set('ru');
});
