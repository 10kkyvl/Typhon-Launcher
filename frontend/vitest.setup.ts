import { locale } from './src/lib/i18n';

Object.defineProperty(globalThis, 'navigator', {
  value: { language: 'ru-RU' },
  configurable: true,
});

locale.set('ru');
