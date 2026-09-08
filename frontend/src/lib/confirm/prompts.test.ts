import { describe, expect, it } from 'vitest';
import { sendLogsPrompt } from './prompts';

describe('sendLogsPrompt', () => {
  it('spells out what is inside the archive, honestly', () => {
    const prompt = sendLogsPrompt();
    expect(prompt.text).toContain('имя пользователя');
    expect(prompt.text).toContain('пути');
    expect(prompt.text).toContain('игр');
    expect(prompt.text).not.toContain('диагностик');
  });

  it('states the retention period as a separate note', () => {
    const prompt = sendLogsPrompt();
    expect(prompt.note).toContain('3 дня');
    expect(prompt.note).toMatch(/удал/);
  });

  it('gives the confirm button its own busy label', () => {
    const prompt = sendLogsPrompt();
    expect(prompt.busy).toBeTruthy();
    expect(prompt.busy).not.toBe(prompt.confirm);
  });
});
