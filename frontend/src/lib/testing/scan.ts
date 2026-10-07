const OPENERS: Record<string, string> = { '(': ')', '[': ']', '{': '}' };

export function balanced(text: string, open: number): string | null {
  const stack: string[] = [];
  for (let i = open; i < text.length; i++) {
    const ch = text[i];
    if (ch === "'" || ch === '"' || ch === '`') {
      const end = skipString(text, i);
      if (end < 0) return null;
      i = end;
      continue;
    }
    if (OPENERS[ch]) {
      stack.push(OPENERS[ch]);
      continue;
    }
    if (ch === ')' || ch === ']' || ch === '}') {
      if (stack.pop() !== ch) return null;
      if (stack.length === 0) return text.slice(open + 1, i);
    }
  }
  return null;
}

function skipString(text: string, start: number): number {
  const quote = text[start];
  for (let i = start + 1; i < text.length; i++) {
    if (text[i] === '\\') {
      i++;
      continue;
    }
    if (quote === '`' && text[i] === '$' && text[i + 1] === '{') {
      const inner = balanced(text, i + 1);
      if (inner === null) return -1;
      i += inner.length + 2;
      continue;
    }
    if (text[i] === quote) return i;
  }
  return -1;
}

export interface Call {
  file: string;
  key: string;
  args: string;
}

export function keyCalls(files: Record<string, string>, names: string[]): Call[] {
  const pattern = new RegExp(
    String.raw`(?<![\w.$])(?:${names.map((name) => name.replace(/\$/g, '\\$')).join('|')})\(\s*(['"` + '`' + String.raw`])([A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_]+)+)\1`,
    'g',
  );
  const calls: Call[] = [];
  for (const [file, text] of Object.entries(files)) {
    for (const match of text.matchAll(pattern)) {
      const open = text.indexOf('(', match.index);
      const args = balanced(text, open);
      if (args === null) continue;
      calls.push({ file, key: match[2], args });
    }
  }
  return calls;
}

export function secondArgument(args: string, key: string): string | null {
  const keyEnd = args.indexOf(key) + key.length + 1;
  const rest = args.slice(keyEnd).trimStart();
  if (!rest.startsWith(',')) return null;
  return rest.slice(1).trim();
}
