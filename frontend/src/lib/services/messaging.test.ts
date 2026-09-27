import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));
import { isMessageAlive, normalizeMessage, type Message } from './messaging';

function message(patch: Partial<Message> = {}): Message {
  return {
    id: 'm1',
    clientId: 'c1',
    senderId: 'a',
    recipientId: 'b',
    text: 'hello',
    createdAt: '2026-09-10T12:00:00.000Z',
    expiresAt: '2026-09-20T12:00:00.000Z',
    reactions: [],
    ...patch,
  };
}

describe('messaging data rules', () => {
  it('filters messages outside the seven day history window', () => {
    const now = Date.parse('2026-09-12T12:00:00.000Z');
    expect(isMessageAlive(message({ createdAt: '2026-09-04T11:59:59.000Z' }), now)).toBe(false);
    expect(isMessageAlive(message({ createdAt: '2026-09-05T12:00:00.000Z' }), now)).toBe(true);
    expect(isMessageAlive(message({ expiresAt: '2026-09-12T11:59:59.000Z' }), now)).toBe(false);
  });

  it('normalizes nullable reactions from generated bindings', () => {
    const value = normalizeMessage(message({ reactions: null as unknown as Message['reactions'] }));
    expect(value.reactions).toEqual([]);
  });
});
