import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';

const RAW: Record<string, MessageKey> = {
  message_too_long: 'social.chatTooLong',
  message_empty: 'social.chatEmpty',
  rate_limited: 'chat.rate_limited',
  muted: 'chat.muted',
  chat_attachment_too_large: 'chat.attachment_too_large',
  chat_attachment_not_image: 'chat.attachment_not_image',
  chat_attachment_invalid: 'chat.attachment_invalid',
};

export function chatErrorText(err: unknown, fallback: MessageKey): string {
  const code = errorCode(err);
  if (code.startsWith('chat.') && hasMessage(code)) return msg(code);
  const raw = (err instanceof Error ? err.message : String(err ?? '')).trim();
  if (Object.hasOwn(RAW, raw)) return msg(RAW[raw]);
  return msg(fallback);
}
