export const NOTE_LIMIT = 1000;
export const NOTE_PREVIEW = 300;

const WORD_BREAK = 0.7;

export interface NotePreview {
  text: string;
  truncated: boolean;
}

export function noteLength(note: string): number {
  return [...note].length;
}

export function trimNote(note: string): string {
  return note.trim();
}

export function notePreview(note: string): NotePreview {
  const chars = [...note];
  if (chars.length <= NOTE_PREVIEW) return { text: note, truncated: false };

  const head = chars.slice(0, NOTE_PREVIEW).join('');
  const stop = head.search(/\s+\S*$/);
  const text = stop >= NOTE_PREVIEW * WORD_BREAK ? head.slice(0, stop) : head;
  return { text: text.trimEnd(), truncated: true };
}
