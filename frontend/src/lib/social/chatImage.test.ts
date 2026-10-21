import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));
import { applyLanguage } from '../i18n';
import { MAX_CHAT_ATTACHMENTS, MAX_CHAT_IMAGE_BYTES } from '../services/messaging';
import {
  appendDrafts,
  attachmentsOf,
  checkChatImage,
  clipboardImages,
  draftFromFile,
  draftFromPicked,
  fileToBase64,
  messagePreview,
  safeImageUrl,
  type ChatAttachmentDraft,
} from './chatImage';

afterEach(() => applyLanguage('ru'));

function draft(key: string): ChatAttachmentDraft {
  return { key, name: `${key}.png`, bytes: 4, encoded: 'AAAA', previewUrl: 'data:image/png;base64,AAAA' };
}

describe('checkChatImage', () => {
  it.each([
    ['image/png', 1, true],
    ['image/jpeg', MAX_CHAT_IMAGE_BYTES, true],
    ['image/webp', 10, true],
    ['image/gif', 10, true],
    ['image/svg+xml', 10, false],
    ['text/plain', 10, false],
    ['', 10, false],
  ])('accepts %s only when it is a supported image', (type, size, ok) => {
    expect(checkChatImage({ type, size }).ok).toBe(ok);
  });

  it('names the reason: type first, then size', () => {
    expect(checkChatImage({ type: 'application/pdf', size: MAX_CHAT_IMAGE_BYTES + 1 })).toEqual({ ok: false, code: 'chat.image_not_image' });
    expect(checkChatImage({ type: 'image/png', size: MAX_CHAT_IMAGE_BYTES + 1 })).toEqual({ ok: false, code: 'chat.image_too_large' });
  });
});

describe('clipboardImages', () => {
  const png = new File([new Uint8Array([1])], 'a.png', { type: 'image/png' });
  const jpg = new File([new Uint8Array([2])], 'b.jpg', { type: 'image/jpeg' });

  it('collects every pasted image file and skips text and empty entries', () => {
    const items = [
      { kind: 'string', type: 'text/plain', getAsFile: () => null },
      { kind: 'file', type: 'image/png', getAsFile: () => png },
      { kind: 'file', type: 'application/pdf', getAsFile: () => new File([], 'c.pdf') },
      { kind: 'file', type: 'image/jpeg', getAsFile: () => null },
      { kind: 'file', type: 'image/jpeg', getAsFile: () => jpg },
    ];
    expect(clipboardImages(items)).toEqual([png, jpg]);
  });

  it('returns nothing without items', () => {
    expect(clipboardImages(null)).toEqual([]);
    expect(clipboardImages(undefined)).toEqual([]);
    expect(clipboardImages([])).toEqual([]);
  });
});

describe('drafts', () => {
  it('encodes a pasted file as base64 and builds a data preview', async () => {
    const file = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], 'shot.png', { type: 'image/png' });
    const made = await draftFromFile(file);
    expect(made.encoded).toBe('iVBORw==');
    expect(made.previewUrl).toBe('data:image/png;base64,iVBORw==');
    expect(made).toMatchObject({ name: 'shot.png', bytes: 4 });
  });

  it('encodes a file larger than one chunk without corrupting it', async () => {
    const bytes = new Uint8Array(0x8000 * 2 + 5).map((_, index) => index % 251);
    const encoded = await fileToBase64(new Blob([bytes]));
    expect(Uint8Array.from(atob(encoded), (char) => char.charCodeAt(0))).toEqual(bytes);
  });

  it('builds a draft from a picked image with its own key', () => {
    const made = draftFromPicked({ name: 'a.gif', mime: 'image/gif', bytes: 4, encoded: 'AAAA' });
    expect(made.previewUrl).toBe('data:image/gif;base64,AAAA');
    expect(made.key).not.toBe(draftFromPicked({ name: 'a.gif', mime: 'image/gif', bytes: 4, encoded: 'AAAA' }).key);
  });

  it('keeps at most four drafts and reports the overflow', () => {
    const full = appendDrafts([draft('1'), draft('2')], [draft('3'), draft('4')]);
    expect(full.drafts.map((item) => item.key)).toEqual(['1', '2', '3', '4']);
    expect(full.overflow).toBe(false);
    expect(MAX_CHAT_ATTACHMENTS).toBe(4);

    const over = appendDrafts([draft('1'), draft('2'), draft('3')], [draft('4'), draft('5')]);
    expect(over.drafts.map((item) => item.key)).toEqual(['1', '2', '3', '4']);
    expect(over.overflow).toBe(true);

    expect(appendDrafts([draft('1'), draft('2'), draft('3'), draft('4')], [draft('5')])).toEqual({
      drafts: [draft('1'), draft('2'), draft('3'), draft('4')],
      overflow: true,
    });
  });
});

describe('safeImageUrl', () => {
  it.each([
    ['https://cdn.example/a.png', 'https://cdn.example/a.png'],
    ['data:image/png;base64,AAAA', 'data:image/png;base64,AAAA'],
    ['data:image/webp;base64,AAAA', 'data:image/webp;base64,AAAA'],
    ['http://cdn.example/a.png', ''],
    ['javascript:alert(1)', ''],
    ['data:text/html;base64,AAAA', ''],
    ['data:image/svg+xml;base64,AAAA', ''],
    ['file:///c:/a.png', ''],
    ['', ''],
  ])('draws %s as %s', (url, expected) => {
    expect(safeImageUrl(url)).toBe(expected);
  });

  it('draws nothing for a missing url', () => {
    expect(safeImageUrl(undefined)).toBe('');
    expect(safeImageUrl(null)).toBe('');
  });
});

describe('messagePreview and attachmentsOf', () => {
  const base = {
    id: 'm', clientId: 'c', senderId: 'a', recipientId: 'b', createdAt: '', expiresAt: '', reactions: [],
  };

  it('shows the text, or the word for a picture when there is none', () => {
    expect(messagePreview({ ...base, text: 'hello' })).toBe('hello');
    expect(messagePreview({ ...base, text: '  ' })).toBe('Изображение');
    applyLanguage('en');
    expect(messagePreview({ ...base, text: '' })).toBe('Image');
    expect(messagePreview(null)).toBe('');
  });

  it('treats a missing attachment list as empty', () => {
    expect(attachmentsOf(undefined)).toEqual([]);
    expect(attachmentsOf({ attachments: null })).toEqual([]);
    expect(attachmentsOf({})).toEqual([]);
  });
});
