import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));
import { applyLanguage, translator } from '../i18n';
import { chatErrorText } from './chatErrors';

afterEach(() => applyLanguage('ru'));

const ru = translator('ru');

describe('chatErrorText', () => {
  it.each([
    'chat.image_too_large',
    'chat.image_not_image',
    'chat.image_unreadable',
    'chat.attachment_too_large',
    'chat.attachment_not_image',
    'chat.attachment_invalid',
    'chat.rate_limited',
    'chat.muted',
  ] as const)('says something specific for %s', (code) => {
    const text = chatErrorText(new Error(`typhon:${code}: backend detail`), 'social.chatSendError');
    expect(text).toBe(ru(code));
    expect(text).not.toBe(ru('social.chatSendError'));
  });

  it.each([
    ['message_too_long', 'social.chatTooLong'],
    ['message_empty', 'social.chatEmpty'],
    ['rate_limited', 'chat.rate_limited'],
    ['muted', 'chat.muted'],
    ['chat_attachment_too_large', 'chat.attachment_too_large'],
    ['chat_attachment_not_image', 'chat.attachment_not_image'],
    ['chat_attachment_invalid', 'chat.attachment_invalid'],
  ] as const)('maps the raw server answer %s', (raw, key) => {
    expect(chatErrorText(new Error(raw), 'social.chatSendError')).toBe(ru(key));
  });

  it('uses the fallback for anything it does not know', () => {
    expect(chatErrorText(new Error('network'), 'social.chatSendError')).toBe(ru('social.chatSendError'));
    expect(chatErrorText(new Error('typhon:other.code: x'), 'social.chatEditError')).toBe(ru('social.chatEditError'));
    expect(chatErrorText(undefined, 'social.chatSendError')).toBe(ru('social.chatSendError'));
  });
});
