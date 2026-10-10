import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { locale } from '../i18n';
import { ru } from '../i18n/catalog/ru';
import { en } from '../i18n/catalog/en';
import { SUPPORT_STATUSES, SUPPORT_TOPICS } from '../services/support';
import { goSources } from '../testing/sources';
import {
  attachLogsByDefault,
  isTopic,
  parseTicketNumber,
  parseTopic,
  statusView,
  topicLabel,
  topicOptions,
} from './topics';

afterEach(() => locale.set('ru'));

function goConstants(prefix: string): string[] {
  const source = goSources['internal/support/model.go'];
  expect(source, 'internal/support/model.go').toBeTypeOf('string');
  return [...source.matchAll(new RegExp(`\\b${prefix}\\w+\\s*=\\s*"([a-z_]+)"`, 'g'))].map((match) => match[1]);
}

describe('topics and statuses against Go', () => {
  it('offers exactly the topics the Go service accepts', () => {
    expect([...SUPPORT_TOPICS].sort()).toEqual(goConstants('Topic').sort());
  });

  it('knows exactly the statuses the Go service returns', () => {
    expect([...SUPPORT_STATUSES].sort()).toEqual(goConstants('Status').sort());
  });
});

describe('topics', () => {
  it('has a label in both languages for each topic', () => {
    for (const language of ['ru', 'en'] as const) {
      locale.set(language);
      const labels = topicOptions().map((option) => option.label);
      expect(labels, language).toHaveLength(SUPPORT_TOPICS.length);
      expect(new Set(labels).size, language).toBe(labels.length);
      for (const label of labels) expect(label.trim()).not.toBe('');
    }
    locale.set('ru');
    expect(topicLabel('install')).toBe(ru['tickets.topicInstall']);
    locale.set('en');
    expect(topicLabel('install')).toBe(en['tickets.topicInstall']);
  });

  it('shows nothing for a topic it does not know', () => {
    expect(topicLabel('spam')).toBe('');
    expect(isTopic('spam')).toBe(false);
    expect(isTopic(undefined)).toBe(false);
  });

  it('attaches the logs by default for install, launch and launcher problems only', () => {
    const on = SUPPORT_TOPICS.filter((topic) => attachLogsByDefault(topic));
    expect(on.sort()).toEqual(['install', 'launch', 'launcher']);
  });

  it('opens on install unless the caller named a valid topic', () => {
    expect(parseTopic(undefined)).toBe('install');
    expect(parseTopic('nonsense')).toBe('install');
    expect(parseTopic('launcher')).toBe('launcher');
    expect(parseTopic('other')).toBe('other');
  });
});

describe('ticket numbers from the route', () => {
  it('reads a positive integer', () => {
    expect(parseTicketNumber('1234')).toBe(1234);
  });

  it('rejects everything else', () => {
    for (const bad of [undefined, '', '0', '-5', '12.5', '1e3', 'abc', '12abc', ' 12', '99999999999999999999']) {
      expect(parseTicketNumber(bad), String(bad)).toBeNull();
    }
  });
});

describe('statusView', () => {
  it('gives each status a label and a tone', () => {
    expect(statusView('open')).toEqual({ kind: 'accent', label: ru['tickets.statusOpen'] });
    expect(statusView('waiting_user')).toEqual({ kind: 'warning', label: ru['tickets.statusWaitingUser'] });
    expect(statusView('resolved')).toEqual({ kind: 'success', label: ru['tickets.statusResolved'] });
  });

  it('shows no badge for a status the launcher does not know', () => {
    expect(statusView('escalated')).toBeNull();
  });
});
