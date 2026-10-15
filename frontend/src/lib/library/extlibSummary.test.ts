import { describe, expect, it } from 'vitest';
import { importErrorText, providerName, summarizeImport } from './extlibSummary';
import type { ExtlibProviderResult, ExtlibSummary } from '../services/extlib';

const provider = (over: Partial<ExtlibProviderResult>): ExtlibProviderResult => ({
  provider: 'steam',
  found: 0,
  added: 0,
  updated: 0,
  skipped: null,
  notInstalled: false,
  error: '',
  ...over,
});

const summary = (over: Partial<ExtlibSummary>): ExtlibSummary => ({
  providers: [],
  added: 0,
  updated: 0,
  metadataErrors: null,
  ...over,
});

describe('summarizeImport', () => {
  it('reports added and updated counts', () => {
    const report = summarizeImport(
      summary({ providers: [provider({ found: 3, added: 2, updated: 1 })], added: 2, updated: 1 }),
    );
    expect(report).toEqual({ text: 'Добавлено 2, обновлено 1', kind: 'success', details: [] });
  });

  it('says no launchers were found when every provider is missing', () => {
    const report = summarizeImport(
      summary({
        providers: [
          provider({ provider: 'steam', notInstalled: true }),
          provider({ provider: 'gog', notInstalled: true }),
          provider({ provider: 'epic', notInstalled: true }),
        ],
      }),
    );
    expect(report.text).toBe('Лаунчеры Steam, GOG Galaxy и Epic не найдены');
    expect(report.kind).toBe('info');
  });

  it('keeps counts when only some launchers are missing', () => {
    const report = summarizeImport(
      summary({
        providers: [provider({ notInstalled: true }), provider({ provider: 'gog', added: 1 })],
        added: 1,
      }),
    );
    expect(report.text).toBe('Добавлено 1, обновлено 0');
    expect(report.kind).toBe('success');
  });

  it('lists provider errors, skipped games and metadata errors', () => {
    const report = summarizeImport(
      summary({
        providers: [
          provider({ provider: 'epic', error: 'нет доступа' }),
          provider({ provider: 'steam', skipped: [{ title: 'Portal', reason: 'папка не найдена' }] }),
        ],
        metadataErrors: ['сеть недоступна'],
      }),
    );
    expect(report.kind).toBe('danger');
    expect(report.details).toEqual([
      'Epic: нет доступа',
      'Steam, «Portal»: папка не найдена',
      'Описание и обложки: сеть недоступна',
    ]);
    expect(report.text).toContain('Добавлено 0, обновлено 0');
  });

  it('shows skipped games without turning the toast into an error', () => {
    const report = summarizeImport(
      summary({ providers: [provider({ skipped: [{ title: 'A', reason: 'r' }] })] }),
    );
    expect(report.kind).toBe('success');
    expect(report.details).toHaveLength(1);
  });

  it('turns the toast into an error when only metadata failed', () => {
    const report = summarizeImport(
      summary({ providers: [provider({ found: 1, added: 1 })], added: 1, metadataErrors: ['x'] }),
    );
    expect(report.kind).toBe('danger');
    expect(report.details).toHaveLength(1);
  });

  it('names providers', () => {
    expect(providerName('gog')).toBe('GOG Galaxy');
    expect(providerName('other')).toBe('other');
  });
});

describe('importErrorText', () => {
  it('translates a busy code', () => {
    expect(importErrorText(new Error('typhon:extlib.busy'))).toBe('поиск игр уже идёт');
  });

  it('explains the non-Windows refusal', () => {
    expect(importErrorText(new Error('typhon:extlib.unsupported'))).toBe(
      'импорт игр из других лаунчеров поддерживается только в Windows',
    );
  });

  it('shows the message of an unknown error', () => {
    expect(importErrorText(new Error('boom'))).toBe('boom');
  });

  it('falls back when there is no message', () => {
    expect(importErrorText(new Error(''))).toBe('Не удалось найти игры других лаунчеров');
    expect(importErrorText(undefined)).toBe('Не удалось найти игры других лаунчеров');
  });
});
