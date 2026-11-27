import { describe, expect, it, vi } from 'vitest';
import { cloudsavesService, emulatorsService, legalService, settingsService, telemetryLogService, themeService } from './settingsDialogsTesting';

const services = import.meta.glob<string>(
  ['../services/cloudsaves.ts', '../services/settings.ts', '../services/theme.ts', '../services/telemetryLog.ts', '../services/legal.ts', '../services/emulators.ts'],
  { query: '?raw', import: 'default', eager: true },
);

/** The exported functions of a service file that call one of its generated bindings (`<Name>Service.`). */
function bindingCalls(source: string): string[] {
  const bindings = [...source.matchAll(/import \{ Service(?: as (\w+))? \} from '[^']*bindings\//g)].map((match) => match[1] ?? 'Service');
  const names: string[] = [];
  let current = '';
  for (const line of source.split('\n')) {
    const declared = /^export (?:async )?function (\w+)/.exec(line);
    if (declared) current = declared[1];
    else if (/^\S/.test(line)) current = '';
    if (current && bindings.some((binding) => line.includes(`${binding}.`)) && !names.includes(current)) names.push(current);
  }
  return names;
}

describe('the whole mocks of the settings dialogs’ services', () => {
  it.each([
    ['../services/cloudsaves.ts', cloudsavesService],
    ['../services/settings.ts', settingsService],
    ['../services/theme.ts', themeService],
    ['../services/telemetryLog.ts', telemetryLogService],
    ['../services/legal.ts', legalService],
    ['../services/emulators.ts', emulatorsService],
  ] as const)('replace every function of %s that calls its binding', (path, factory) => {
    const calling = bindingCalls(services[path]);
    expect(calling.length).toBeGreaterThan(0);
    const real = Object.fromEntries(calling.map((name) => [name, () => 'real']));
    const mocked = factory(real) as Record<string, unknown>;
    expect(calling.filter((name) => !vi.isMockFunction(mocked[name]))).toEqual([]);
  });

  it('answer nothing the test did not ask for', async () => {
    const mocked = cloudsavesService({});
    await expect(mocked.cloudForget()).rejects.toThrow('cloudForget is not answered by this test');
  });
});
