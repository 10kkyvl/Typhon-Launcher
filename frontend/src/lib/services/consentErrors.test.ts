import { describe, expect, it } from 'vitest';
import { consentErrorText } from './consentErrors';

describe('consentErrorText', () => {
  it('maps the save-failed code to its own message', () => {
    expect(consentErrorText(new Error('typhon:settings.consent_save_failed: create config dir: denied'))).toBe(
      'Не удалось сохранить ответ: нет доступа к папке настроек. Проверьте права и попробуйте ещё раз.',
    );
  });

  it('never shows raw go error text', () => {
    const fallback = 'Не удалось сохранить ответ. Попробуйте ещё раз.';
    expect(consentErrorText(new Error('save telemetry consent: create config dir: permission denied'))).toBe(fallback);
    expect(consentErrorText(undefined)).toBe(fallback);
  });
});
