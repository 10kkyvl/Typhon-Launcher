export type ChatTextPart = { text: string; href?: string };

const URL_RE = /https?:\/\/[^\s<]+/gi;

export function chatTextParts(text: string): ChatTextPart[] {
  const parts: ChatTextPart[] = [];
  let cursor = 0;
  for (const match of text.matchAll(URL_RE)) {
    const index = match.index ?? 0;
    const raw = match[0];
    if (index > cursor) parts.push({ text: text.slice(cursor, index) });
    const trailing = raw.match(/[),.!?;:]+$/)?.[0] ?? '';
    const href = trailing ? raw.slice(0, -trailing.length) : raw;
    parts.push({ text: href, href });
    if (trailing) parts.push({ text: trailing });
    cursor = index + raw.length;
  }
  if (cursor < text.length) parts.push({ text: text.slice(cursor) });
  return parts.length > 0 ? parts : [{ text }];
}
