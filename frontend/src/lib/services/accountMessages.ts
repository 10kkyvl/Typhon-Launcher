import { AccountError } from './account';

const MESSAGES: Record<string, string> = {
  invalid_credentials: 'Неверный email, имя пользователя или пароль',
  username_taken: 'Это имя пользователя уже занято',
  email_taken: 'На этот email уже зарегистрирован аккаунт',
  invalid_username: '3–24 символа: латиница, цифры, _ и точка (не в начале и не в конце)',
  invalid_display_name: 'От 1 до 32 символов',
  invalid_email: 'Введите корректный email',
  invalid_password: 'От 8 до 128 символов',
  email_immutable: 'Email пока нельзя изменить',
  launcher_outdated: 'Версия лаунчера устарела, обновите Typhon, чтобы пользоваться аккаунтом',
  no_changes: 'Нечего сохранять',
  avatar_too_large: 'Файл больше 10 МБ',
  unsupported_avatar: 'Поддерживаются PNG, JPEG и WebP',
  invalid_avatar: 'Не удалось прочитать изображение',
  invalid_profile: 'Некорректные настройки профиля',
  invalid_bio: 'Био не длиннее 150 символов',
  unauthenticated: 'Сессия истекла, войдите заново',
  network_error: 'Нет связи с сервером',
  rate_limited: 'Слишком много попыток, подождите немного и повторите',
  bad_request: 'Сервер отклонил запрос',
  request_blocked: 'Запрос отклонён',
  user_not_found: 'Пользователь не найден',
  already_friends: 'Вы уже друзья',
  friend_limit: 'Достигнут лимит',
  request_limit: 'Достигнут лимит',
  block_limit: 'Достигнут лимит',
  friend_self: 'Нельзя добавить самого себя',
  internal: 'Ошибка на стороне сервера, попробуйте позже',
  server_error: 'Сервер ответил непонятным образом, попробуйте позже',
};

export function accountMessage(code: string, fallback = 'Не удалось выполнить операцию'): string {
  return MESSAGES[code] ?? fallback;
}

export function accountErrorText(err: unknown, fallback?: string): string {
  const code = err instanceof AccountError ? err.code : '';
  return accountMessage(code, fallback);
}

export function accountErrorField(err: unknown): string {
  return err instanceof AccountError ? err.field : '';
}
