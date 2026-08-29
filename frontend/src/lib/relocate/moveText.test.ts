import { describe, expect, it } from 'vitest';
import { movePercent, moveSummary, stageLabel } from './moveText';
import { bytesSize } from '../utils/format';
import type { MoveJob } from '../services/relocate';

const job = (over: Partial<MoveJob> = {}): MoveJob => ({
  id: 'job-1',
  scope: 'game',
  stage: 'copy',
  title: 'Тестовая игра',
  source: 'C:\\Games\\Test',
  target: 'D:\\Games\\Test',
  staging: 'D:\\Games\\Test.staging',
  renamed: false,
  totalBytes: 0,
  copiedBytes: 0,
  phase: '',
  currentFile: '',
  startedAt: '',
  updatedAt: '',
  ...over,
});

describe('stageLabel', () => {
  it.each([
    ['prepare', 'Подготовка'],
    ['copy', 'Копирование'],
    ['verify', 'Проверка'],
    ['commit', 'Завершение'],
    ['repoint', 'Перепривязка'],
    ['cleanup', 'Уборка'],
    ['done', 'Готово'],
    ['failed', 'Не удалось'],
    ['cancelled', 'Отменено'],
  ] as const)('labels %s as %s', (stage, label) => {
    expect(stageLabel(stage)).toBe(label);
  });
});

describe('movePercent', () => {
  it('divides copied by total', () => {
    expect(movePercent(job({ copiedBytes: 41, totalBytes: 100 }))).toBe(41);
  });

  it('does not divide by zero', () => {
    expect(movePercent(job({ copiedBytes: 0, totalBytes: 0 }))).toBe(0);
  });

  it('clamps to the 0..100 range', () => {
    expect(movePercent(job({ copiedBytes: 150, totalBytes: 100 }))).toBe(100);
    expect(movePercent(job({ copiedBytes: -10, totalBytes: 100 }))).toBe(0);
  });
});

describe('moveSummary', () => {
  it('reports copy progress with bytes', () => {
    const totalBytes = 83 * 1024 ** 3;
    const copiedBytes = Math.round(totalBytes * 0.41);
    const summary = moveSummary(job({ stage: 'copy', copiedBytes, totalBytes }));
    expect(summary).toBe(`Копирование 41% · ${bytesSize(copiedBytes)} из ${bytesSize(totalBytes)}`);
  });

  it('does not look stuck at 100% once verification starts', () => {
    const totalBytes = 10 * 1024 ** 3;
    const summary = moveSummary(
      job({ stage: 'verify', phase: 'проверка', copiedBytes: totalBytes, totalBytes }),
    );
    expect(summary).toBe('Проверка: проверка');
    expect(summary).not.toContain('100%');
  });

  it('falls back to the stage label when there is no phase', () => {
    expect(moveSummary(job({ stage: 'prepare', phase: '' }))).toBe('Подготовка');
    expect(moveSummary(job({ stage: 'done', phase: '' }))).toBe('Готово');
  });
});
