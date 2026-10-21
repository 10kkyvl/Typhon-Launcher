import { afterEach, describe, expect, it, vi } from 'vitest';

const backend = vi.hoisted(() => ({ inWails: false }));
const binding = vi.hoisted(() => ({
  Send: vi.fn(),
  PickChatImages: vi.fn(),
  UploadChatImage: vi.fn(),
}));

vi.mock('./backend', () => ({ get inWails() { return backend.inWails; } }));
vi.mock('../../../bindings/typhon/internal/messaging', () => ({ Service: binding }));
import { applyLanguage } from '../i18n';
import { isMessageAlive, normalizeMessage, pickChatImages, send, uploadChatImage, type Message } from './messaging';

afterEach(() => {
  backend.inWails = false;
  vi.clearAllMocks();
  applyLanguage('ru');
});

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
  it('turns missing attachments from generated bindings into an empty list', () => {
    expect(normalizeMessage(message({ attachments: null })).attachments).toEqual([]);
    expect(normalizeMessage(message()).attachments).toEqual([]);
  });
});

describe('chat image calls', () => {
  it('passes the attachment ids as the fourth argument of Send', async () => {
    backend.inWails = true;
    binding.Send.mockResolvedValue(message());
    await send('peer', 'client', 'hi', ['a1', 'a2']);
    await send('peer', 'client', 'hi');
    expect(binding.Send).toHaveBeenNthCalledWith(1, 'peer', 'client', 'hi', ['a1', 'a2']);
    expect(binding.Send).toHaveBeenNthCalledWith(2, 'peer', 'client', 'hi', []);
  });

  it('asks for the pictures in the current language and drops cancelled entries', async () => {
    backend.inWails = true;
    applyLanguage('en');
    const picked = { name: 'a.png', mime: 'image/png', bytes: 4, encoded: 'AAAA' };
    binding.PickChatImages.mockResolvedValueOnce([picked, { name: '', mime: '', bytes: 0, encoded: '' }]);
    expect(await pickChatImages()).toEqual([picked]);
    expect(binding.PickChatImages).toHaveBeenCalledWith('en');
    binding.PickChatImages.mockResolvedValueOnce(null);
    expect(await pickChatImages()).toEqual([]);
  });

  it('uploads one encoded picture and returns the server attachment', async () => {
    backend.inWails = true;
    const attachment = { id: 'a1', url: 'https://cdn.example/a1.png', width: 2, height: 2, bytes: 4 };
    binding.UploadChatImage.mockResolvedValueOnce(attachment);
    expect(await uploadChatImage('AAAA')).toEqual(attachment);
    expect(binding.UploadChatImage).toHaveBeenCalledWith('AAAA');
  });

  it('refuses to pick, upload or send outside the app', async () => {
    await expect(pickChatImages()).rejects.toThrow('messaging_unavailable');
    await expect(uploadChatImage('AAAA')).rejects.toThrow('messaging_unavailable');
    await expect(send('peer', 'client', 'hi', ['a1'])).rejects.toThrow('messaging_unavailable');
    expect(binding.Send).not.toHaveBeenCalled();
  });
});
