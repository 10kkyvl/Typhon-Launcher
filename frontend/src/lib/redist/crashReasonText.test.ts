import { describe, expect, it } from 'vitest';
import { ru } from '../i18n/catalog/ru';
import { en } from '../i18n/catalog/en';

describe('crash reason text', () => {
  for (const [name, catalog] of [['ru', ru], ['en', en]] as const) {
    it(`${name}: reasons carry no trailing period, the template adds it`, () => {
      const all = catalog as Record<string, unknown>;
      const template = all['redist.crashReason'];
      expect(typeof template).toBe('string');
      const reasons = Object.entries(all).filter(([key]) => key.startsWith('redist.hint') || key.startsWith('crash.reason.'));
      expect(reasons.length).toBeGreaterThan(0);
      for (const [key, text] of reasons) {
        expect(typeof text, key).toBe('string');
        const line = (template as string).replace('{reason}', text as string);
        expect(line, key).not.toContain('..');
        expect(line.endsWith('.'), key).toBe(true);
      }
    });
  }
});
