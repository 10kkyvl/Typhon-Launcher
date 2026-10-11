import { afterEach, describe, expect, it } from 'vitest';
import { locale } from '../i18n/locale';
import { sendLogsPrompt } from '../confirm/prompts';
import { ticketLogsPrompt } from './logsPrompt';

describe('ticketLogsPrompt', () => {
  afterEach(() => locale.set('ru'));

  it.each([
    ['ru', '30 дней'],
    ['en', '30 days'],
  ] as const)('states how long the archive is kept in %s', (language, period) => {
    locale.set(language);
    const prompt = ticketLogsPrompt();
    expect(prompt.note).toContain(period);
    expect(prompt.note).toMatch(language === 'ru' ? /не решено|решени/ : /resolved/);
  });

  it.each(['ru', 'en'] as const)('never repeats the three-day period of the manual upload in %s', (language) => {
    locale.set(language);
    const prompt = ticketLogsPrompt();
    const all = `${prompt.title} ${prompt.text} ${prompt.note ?? ''}`;
    expect(all).not.toMatch(/3 дня|3 days|три дня|three days/i);
    expect(prompt.note).not.toBe(sendLogsPrompt().note);
  });

  it.each(['ru', 'en'] as const)('says what stays in the archive and promises no clean scrub in %s', (language) => {
    locale.set(language);
    const prompt = ticketLogsPrompt();
    expect(prompt.text).not.toMatch(/очищенные от личных данных|personal data removed/i);
    expect(prompt.text).toMatch(language === 'ru' ? /Названия игр/ : /names of games/);
    expect(prompt.text).toMatch(language === 'ru' ? /не даёт гарантий/ : /not guaranteed/);
    expect(prompt.text).toMatch(language === 'ru' ? /Политик/ : /Policy/i);
  });

  it('has its own confirm and busy labels', () => {
    const prompt = ticketLogsPrompt();
    expect(prompt.confirm).toBeTruthy();
    expect(prompt.busy).toBeTruthy();
    expect(prompt.busy).not.toBe(prompt.confirm);
  });
});
