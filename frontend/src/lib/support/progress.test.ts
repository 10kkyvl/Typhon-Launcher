import { describe, expect, it } from 'vitest';
import { ru } from '../i18n/catalog/ru';
import { logsStageText, uploadPercent } from './progress';

describe('uploadPercent', () => {
  it('is the share of bytes sent, capped at 100', () => {
    expect(uploadPercent({ state: 'sending', sentBytes: 25, totalBytes: 100 })).toBe(25);
    expect(uploadPercent({ state: 'sending', sentBytes: 300, totalBytes: 100 })).toBe(100);
  });

  it('is zero until the total is known', () => {
    expect(uploadPercent({ state: 'sending' })).toBe(0);
    expect(uploadPercent({ state: 'sending', sentBytes: 10, totalBytes: 0 })).toBe(0);
  });
});

describe('logsStageText', () => {
  it('says nothing before the upload starts and after it ends', () => {
    expect(logsStageText(null)).toBe('');
    expect(logsStageText({ state: 'success' })).toBe('');
    expect(logsStageText({ state: 'error' })).toBe('');
  });

  it('names each stage', () => {
    expect(logsStageText({ state: 'preparing' })).toBe(ru['settings.aboutLogsPreparingEllipsis']);
    expect(logsStageText({ state: 'waiting' })).toBe(ru['settings.aboutLogsWaitingEllipsis']);
    expect(logsStageText({ state: 'sending' })).toBe(ru['settings.aboutLogsSendingEllipsis']);
    expect(logsStageText({ state: 'sending', sentBytes: 50, totalBytes: 200 })).toBe('Отправляем: 25%');
  });
});
