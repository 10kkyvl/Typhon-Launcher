import { msg, type MessageKey } from '../i18n';

// Also handles warnings persisted by earlier launchers, whose feed parser emitted Russian text.
const warnings: [RegExp, MessageKey][] = [
  [/^фид обрезан до (\d+) записей \(превышен лимит\)$/, 'errSources.warningTruncated'],
  [/^(\d+) записей пропущено из-за некорректного заголовка/, 'errSources.warningTitle'],
  [/^(\d+) записей пропущено: нет валидного URI/, 'errSources.warningNoUri'],
  [/^(\d+) URI пропущено: поддерживается только magnet/, 'errSources.warningNonMagnet'],
  [/^(\d+) раздач доступны только по прямой ссылке — прямые ссылки пока не поддерживаются/, 'errSources.warningHttpOnly'],
  [/^(\d+) URI пропущено: превышена максимальная длина/, 'errSources.warningLongUri'],
  [/^(\d+) записей: список URI обрезан до лимита (\d+)/, 'errSources.warningUriLimit'],
  [/^(\d+) записей: отрицательный размер файла обнулён/, 'errSources.warningNegativeSize'],
  [/^(\d+) записей: не удалось разобрать размер файла/, 'errSources.warningBadSize'],
  [/^(\d+) записей: не удалось разобрать дату публикации/, 'errSources.warningBadDate'],
  [/^(\d+) дублирующихся записей объединено/, 'errSources.warningDuplicates'],
  [/^(\d+) патчей пропущено: не указаны fromVersion\/toVersion/, 'errSources.warningBadPatch'],
  [/^(\d+) записей с неизвестным типом обработано как обычный релиз/, 'errSources.warningUnknownType'],
];

export function sourceWarningText(raw: string): string {
  for (const [pattern, key] of warnings) {
    const match = pattern.exec(raw);
    if (match) return msg(key, { count: Number(match[1]), limit: Number(match[2] ?? 0) });
  }
  return raw;
}
