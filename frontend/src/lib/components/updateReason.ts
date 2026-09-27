import type { MessageKey } from '../i18n/catalog/ru';
// Legacy strings remain readable until the next availability refresh.
export function updateReasonKey(reason: string): MessageKey {
  if (reason.startsWith('edition: ')) return 'ui.updateDifferentEdition';
  if (reason.startsWith('language: ')) return 'ui.updateDifferentLanguage';
  const keys: Record<string, MessageKey> = {
    new_distribution_revision: 'ui.newDistributionRevisionReason',
    versions_not_comparable: 'ui.versionsNotComparable',
    'низкая уверенность в сопоставлении версий': 'ui.versionsNotComparable',
    release_much_smaller: 'ui.updateReleaseMuchSmaller',
    'размер раздачи сильно меньше установленной игры': 'ui.updateReleaseMuchSmaller',
    different_edition: 'ui.updateDifferentEdition',
    edition_unverified: 'ui.updateEditionUnverified',
    edition_unknown: 'ui.updateEditionUnknown',
    'edition unknown': 'ui.updateEditionUnknown',
    different_language: 'ui.updateDifferentLanguage',
  };
  return keys[reason] ?? 'ui.updateCompatibilityReview';
}
