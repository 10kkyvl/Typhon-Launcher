import type { Message } from '../../types';
import type { ErrMetadataKey } from '../ru/errMetadata';

export const errMetadata: Record<ErrMetadataKey, Message> = {
  'errMetadata.fallback': 'Could not complete the metadata operation',
  'errMetadata.catalogGameNotFound': 'Game not found',
  'errMetadata.catalogNoIgdbId': 'IGDB ID is not specified',
  'errMetadata.catalogNoTitle': 'Enter the game title',
  'errMetadata.catalogDuplicateId': 'A game with this ID already exists',
  'errMetadata.catalogNothingToLearn': 'Nothing to remember',
  'errMetadata.catalogNoProviderId': 'Provider ID is not specified',
  'errMetadata.catalogNoTimestamp': 'Metadata update time is not specified',
  'errMetadata.catalogSaveFailed': 'Failed to save changes to the game catalog',
  'errMetadata.metadataNoGameId': 'No game specified',
  'errMetadata.metadataNotStarted': 'The metadata service is not running',
  'errMetadata.metadataEmptyQuery': 'Empty search query',
  'errMetadata.metadataNoTitle': 'The provider returned a game with no title',
  'errMetadata.metadataBusy': "This game's metadata is already being refreshed",
  'errMetadata.metadataNotConfigured': 'The metadata provider is not configured',
  'errMetadata.metadataAmbiguousMatch': 'No single unambiguous match was found',
  'errMetadata.metadataNoMatch': 'The metadata provider has no match for this game',
  'errMetadata.metadataRateLimited': 'The metadata server rate-limited requests',
  'errMetadata.metadataSaveFailed': 'Failed to save metadata',
};
