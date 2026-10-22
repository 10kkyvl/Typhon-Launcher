import { msg } from '../i18n';
import {
  CHAT_IMAGE_MIMES,
  MAX_CHAT_ATTACHMENTS,
  MAX_CHAT_IMAGE_BYTES,
  type Attachment,
  type ChatImage,
  type Message,
} from '../services/messaging';

export type ChatImageRejection = 'chat.image_not_image' | 'chat.image_too_large';

export type ChatImageCheck = { ok: true } | { ok: false; code: ChatImageRejection };

export interface ChatAttachmentDraft {
  key: string;
  name: string;
  bytes: number;
  encoded: string;
  previewUrl: string;
  uploaded?: Attachment;
}

interface ClipboardEntry {
  kind: string;
  type: string;
  getAsFile(): File | null;
}

function allowedMime(mime: string): boolean {
  return (CHAT_IMAGE_MIMES as readonly string[]).includes(mime);
}

export function checkChatImage(file: { type: string; size: number }): ChatImageCheck {
  if (!allowedMime(file.type)) return { ok: false, code: 'chat.image_not_image' };
  if (file.size > MAX_CHAT_IMAGE_BYTES) return { ok: false, code: 'chat.image_too_large' };
  return { ok: true };
}

export function clipboardImages(items: ArrayLike<ClipboardEntry> | null | undefined): File[] {
  const files: File[] = [];
  if (!items) return files;
  for (let index = 0; index < items.length; index += 1) {
    const item = items[index];
    if (item.kind !== 'file' || !item.type.startsWith('image/')) continue;
    const file = item.getAsFile();
    if (file) files.push(file);
  }
  return files;
}

export async function fileToBase64(file: Blob): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = '';
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

export function draftFromPicked(image: ChatImage): ChatAttachmentDraft {
  return {
    key: crypto.randomUUID(),
    name: image.name,
    bytes: image.bytes,
    encoded: image.encoded,
    previewUrl: `data:${image.mime};base64,${image.encoded}`,
  };
}

export async function draftFromFile(file: File): Promise<ChatAttachmentDraft> {
  const encoded = await fileToBase64(file);
  return {
    key: crypto.randomUUID(),
    name: file.name || 'image',
    bytes: file.size,
    encoded,
    previewUrl: `data:${file.type};base64,${encoded}`,
  };
}

export function appendDrafts(
  current: ChatAttachmentDraft[],
  incoming: ChatAttachmentDraft[],
): { drafts: ChatAttachmentDraft[]; overflow: boolean } {
  const room = Math.max(0, MAX_CHAT_ATTACHMENTS - current.length);
  return { drafts: [...current, ...incoming.slice(0, room)], overflow: incoming.length > room };
}

export function attachmentsOf(message: Pick<Message, 'attachments'> | null | undefined): Attachment[] {
  return message?.attachments ?? [];
}

export function safeImageUrl(url: string | undefined | null): string {
  if (!url) return '';
  if (url.startsWith('https://')) return url;
  return CHAT_IMAGE_MIMES.some((mime) => url.startsWith(`data:${mime};base64,`)) ? url : '';
}

export function messagePreview(message: Message | null | undefined): string {
  if (!message) return '';
  if (message.text.trim() !== '') return message.text;
  return msg('social.chatImageLabel');
}
