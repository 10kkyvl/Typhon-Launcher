import { errorCode, hasMessage, msg } from '../i18n';

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}
