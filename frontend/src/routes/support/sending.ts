import { msg } from '../../lib/i18n';
import type { TextAreaCounter } from '../../lib/components/textArea';
import { bodyLength } from '../../lib/support/draft';
import { onMacKeyboard } from '../../lib/utils/shortcut';

/** The keys that send a support text, as the keyboard labels them. */
export function sendKeys(mac: boolean = onMacKeyboard()): string {
  return mac ? '⌘ Enter' : 'Ctrl Enter';
}

/** Ctrl+Enter or Cmd+Enter in a support text: send it. A plain Enter is a new line; a held key repeats nothing. */
export function isSendShortcut(event: KeyboardEvent): boolean {
  return event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.shiftKey && !event.altKey && !event.isComposing && !event.repeat;
}

/** Enter alone in a one-line support field: send the form. */
export function isPlainEnter(event: KeyboardEvent): boolean {
  return event.key === 'Enter' && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey && !event.isComposing && !event.repeat;
}

/** The counter of a support text, counted as Go's limit counts it (trimmed, line breaks folded). */
export function bodyCounter(max: number): TextAreaCounter {
  return {
    max,
    measure: bodyLength,
    text: (count, limit) => msg('tickets.counter', { count, max: limit }),
    limitText: (count, limit) => {
      if (count > limit) return msg('tickets.counterOver', { count: count - limit });
      if (count === limit) return msg('tickets.counterAt');
      return msg('tickets.counterNear', { count: limit - count });
    },
  };
}
