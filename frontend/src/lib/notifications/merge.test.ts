import { describe, expect, it } from 'vitest';
import { mergeNotifications, type LiveEntry } from './merge';

interface Item extends LiveEntry {
  id: string;
}

const item = (over: Partial<Item> = {}): Item => ({ id: 'n1', terminal: false, ...over });

describe('mergeNotifications', () => {
  it('hides a terminal live entry once a history record shares its refId', () => {
    const live = [item({ id: 'update-error:game1', refId: 'game1', terminal: true })];
    const history = [{ refId: 'game1' }];
    expect(mergeNotifications(live, history)).toEqual([]);
  });

  it('keeps a terminal live entry when no history record matches its refId', () => {
    const live = [item({ id: 'update-error:game1', refId: 'game1', terminal: true })];
    const history = [{ refId: 'game2' }];
    expect(mergeNotifications(live, history)).toEqual(live);
  });

  it('never hides a non-terminal live entry, even with a matching history refId', () => {
    const live = [item({ id: 'download:d1', refId: 'd1', terminal: false })];
    const history = [{ refId: 'd1' }];
    expect(mergeNotifications(live, history)).toEqual(live);
  });

  it('keeps a terminal live entry with no refId of its own', () => {
    const live = [item({ id: 'source:s1', terminal: true })];
    const history = [{ refId: 's1' }];
    expect(mergeNotifications(live, history)).toEqual(live);
  });

  it('ignores history records without a refId', () => {
    const live = [item({ id: 'install:i1', refId: 'i1', terminal: true })];
    const history = [{ refId: undefined }];
    expect(mergeNotifications(live, history)).toEqual(live);
  });

  it('passes through an empty live list', () => {
    expect(mergeNotifications([], [{ refId: 'x' }])).toEqual([]);
  });
});
