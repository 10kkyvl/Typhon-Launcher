<script lang="ts" module>
  export interface ReadingPart {
    kind: 'num' | 'unit' | 'text';
    text: string;
  }

  // Digits with the group and decimal separators a locale writes inside a number; a minus sign (−) and a
  // hyphen at the start of a word lead it.
  const NUMBER = /(?:−|(?<![\p{L}\p{N}])-)?\d+(?:[.,  ]\d+)*/gu;
  const GAP = /^[   ]?/;

  /**
   * Cuts a formatted reading into numbers, the units right after them, and the text around. A unit is only
   * a word `units` lists (or `%`), standing alone after its number: "3 из 12 файлов" has no unit.
   */
  export function readingParts(text: string, units: readonly string[]): ReadingPart[] {
    const known = [...units, '%'].filter(Boolean).sort((a, b) => b.length - a.length);
    const parts: ReadingPart[] = [];
    const push = (kind: ReadingPart['kind'], chunk: string) => {
      if (!chunk) return;
      const last = parts.at(-1);
      if (last && last.kind === 'text' && kind === 'text') last.text += chunk;
      else parts.push({ kind, text: chunk });
    };
    let at = 0;
    for (const match of text.matchAll(NUMBER)) {
      const start = match.index;
      if (start < at) continue;
      push('text', text.slice(at, start));
      push('num', match[0]);
      at = start + match[0].length;
      const rest = text.slice(at);
      const gap = GAP.exec(rest)?.[0] ?? '';
      const unit = known.find((name) => {
        const from = name === '%' ? 0 : gap.length;
        if (!rest.startsWith(name, from)) return false;
        const after = rest.slice(from + name.length);
        return !/^[\p{L}\p{N}]/u.test(after);
      });
      if (unit) {
        const from = unit === '%' ? 0 : gap.length;
        push('text', rest.slice(0, from));
        push('unit', unit);
        at += from + unit.length;
      }
    }
    push('text', text.slice(at));
    return parts;
  }
</script>

<script lang="ts">
  import { t } from '../i18n';

  /**
   * A reading: a speed, a size, a share or a time left, given already formatted (utils/format.ts). Its
   * numbers are set in tabular figures, so a value that changes every second does not jitter, and each
   * unit is set apart from its number, smaller. The text is the string given, unchanged: assistive tech,
   * search and tests read the same words. Size and colour come from where it stands; the unit is
   * `--reading-unit` of that size.
   */
  let { text }: { text: string } = $props();

  const UNIT_KEYS = ['units.b', 'units.kb', 'units.mb', 'units.gb', 'units.tb', 'units.kbs', 'units.mbs', 'units.hour', 'units.minute', 'units.second'] as const;

  const parts = $derived(
    readingParts(
      text,
      UNIT_KEYS.map((key) => $t(key)),
    ),
  );
</script>

<!-- No whitespace between the spans: it would print as a gap inside the reading. -->
<span class="reading">{#each parts as part, i (i)}{#if part.kind === 'num'}<span class="num">{part.text}</span>{:else if part.kind === 'unit'}<span class="unit">{part.text}</span>{:else}{part.text}{/if}{/each}</span>

<style>
  .reading {
    font-variant-numeric: tabular-nums;
  }

  .unit {
    font-size: var(--reading-unit, 0.8em);
  }
</style>
