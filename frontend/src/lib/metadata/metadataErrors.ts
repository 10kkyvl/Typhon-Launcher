import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

export const REASONS: Record<string, MessageKey> = {
  'catalog.game_not_found': 'errMetadata.catalogGameNotFound',
  'catalog.no_igdb_id': 'errMetadata.catalogNoIgdbId',
  'catalog.no_title': 'errMetadata.catalogNoTitle',
  'catalog.duplicate_id': 'errMetadata.catalogDuplicateId',
  'catalog.nothing_to_learn': 'errMetadata.catalogNothingToLearn',
  'catalog.no_provider_id': 'errMetadata.catalogNoProviderId',
  'catalog.no_timestamp': 'errMetadata.catalogNoTimestamp',
  'catalog.save_failed': 'errMetadata.catalogSaveFailed',
  'metadata.no_game_id': 'errMetadata.metadataNoGameId',
  'metadata.not_started': 'errMetadata.metadataNotStarted',
  'metadata.empty_query': 'errMetadata.metadataEmptyQuery',
  'metadata.no_title': 'errMetadata.metadataNoTitle',
  'metadata.busy': 'errMetadata.metadataBusy',
  'metadata.not_configured': 'errMetadata.metadataNotConfigured',
  'metadata.ambiguous_match': 'errMetadata.metadataAmbiguousMatch',
  'metadata.no_match': 'errMetadata.metadataNoMatch',
  'metadata.rate_limited': 'errMetadata.metadataRateLimited',
  'metadata.save_failed': 'errMetadata.metadataSaveFailed',
};

export function metadataErrorText(err: unknown, fallback: string = msg('errMetadata.fallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}
