const RESERVED_TOKEN_NAMES = new Set(['--ui-scale']);

export function validateTokenName(name: string, allowed: Set<string> | string[]): string | null {
  if (name.length === 0) return 'Имя токена не может быть пустым';
  if (RESERVED_TOKEN_NAMES.has(name)) return `Токен «${name}» управляется настройками интерфейса`;
  const set = allowed instanceof Set ? allowed : new Set(allowed);
  if (!set.has(name)) return `Неизвестный токен «${name}»`;
  return null;
}

const FORBIDDEN_VALUE_SUBSTRINGS = [';', '{', '}', '/*', '\\', 'url(', '@', 'expression('];

export function validateTokenValue(value: string): string | null {
  if (value.length === 0) return 'Значение не может быть пустым';
  if (value.length > 120) return 'Значение длиннее 120 символов';
  const lower = value.toLowerCase();
  for (const substring of FORBIDDEN_VALUE_SUBSTRINGS) {
    if (lower.includes(substring)) return `Значение содержит запрещённую последовательность «${substring}»`;
  }
  return null;
}

const CSS_MAX_BYTES = 32 * 1024;

const FORBIDDEN_CSS_SUBSTRINGS = [
  '@import',
  'url(',
  'expression(',
  'javascript:',
  'behavior:',
  '-moz-binding',
  '</style',
  '--ui-scale',
];

export function validateCss(css: string): string | null {
  if (new TextEncoder().encode(css).length > CSS_MAX_BYTES) return 'CSS длиннее 32 КиБ';
  const lower = css.toLowerCase();
  for (const substring of FORBIDDEN_CSS_SUBSTRINGS) {
    if (lower.includes(substring)) return `CSS содержит запрещённую конструкцию «${substring}»`;
  }
  let balance = 0;
  for (const char of css) {
    if (char === '{') balance += 1;
    else if (char === '}') {
      balance -= 1;
      if (balance < 0) return 'Лишняя закрывающая фигурная скобка в CSS';
    }
  }
  if (balance !== 0) return 'Незакрытая фигурная скобка в CSS';
  return null;
}
