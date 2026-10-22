export const errChat = {
  'chat.image_too_large': 'Изображение больше 8 МБ. Выберите файл поменьше.',
  'chat.image_not_image': 'Нужен файл PNG, JPEG, WebP или GIF.',
  'chat.image_unreadable': 'Не удалось прочитать файл. Проверьте, что он доступен.',
  'chat.attachment_too_large': 'Сервер отклонил изображение: оно слишком большое.',
  'chat.attachment_not_image': 'Сервер отклонил файл: это не изображение.',
  'chat.attachment_invalid': 'Сервер отклонил изображение: файл повреждён.',
  'chat.rate_limited': 'Слишком много отправок подряд. Подождите немного и повторите.',
  'chat.too_many_attachments': 'В сообщение можно добавить не больше 4 картинок.',
  'chat.muted': 'Отправка в чат временно отключена.',
} as const;

export type ErrChatKey = keyof typeof errChat;
