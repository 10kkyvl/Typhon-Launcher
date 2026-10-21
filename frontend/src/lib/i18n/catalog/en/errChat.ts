import type { Message } from '../../types';
import type { ErrChatKey } from '../ru/errChat';

export const errChat: Record<ErrChatKey, Message> = {
  'chat.image_too_large': 'The image is larger than 8 MB. Choose a smaller file.',
  'chat.image_not_image': 'A PNG, JPEG, WebP or GIF file is required.',
  'chat.image_unreadable': 'Could not read the file. Check that it is accessible.',
  'chat.attachment_too_large': 'The server rejected the image: it is too large.',
  'chat.attachment_not_image': 'The server rejected the file: it is not an image.',
  'chat.attachment_invalid': 'The server rejected the image: the file is damaged.',
  'chat.rate_limited': 'Too many sends in a row. Wait a moment and try again.',
  'chat.too_many_attachments': 'A message can contain up to 4 images.',
  'chat.muted': 'Sending to chat is temporarily disabled.',
};
