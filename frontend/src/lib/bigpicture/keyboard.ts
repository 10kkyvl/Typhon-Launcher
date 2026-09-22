export function appendText(value: string, text: string, maxLength = 300): string {
  return Array.from(value + text).slice(0, Math.max(0, maxLength)).join('');
}

export function eraseCharacter(value: string): string {
  return Array.from(value).slice(0, -1).join('');
}

export const keyboardRows = {
  en: ['1234567890', 'qwertyuiop', 'asdfghjkl', 'zxcvbnm', "-_./\\:@!?'", '&=+%#()[],;'],
  ru: ['1234567890', 'йцукенгшщзхъ', 'фывапролджэ', 'ячсмитьбюё', "-_./\\:@!?'", '&=+%#()[],;'],
} as const;
