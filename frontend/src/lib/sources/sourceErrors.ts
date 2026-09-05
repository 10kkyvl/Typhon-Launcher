import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

export const REASONS: Record<string, MessageKey> = {
  'sources.source_not_found': 'errSources.srcSourceNotFound',
  'sources.source_busy': 'errSources.srcSourceBusy',
  'sources.source_disabled': 'errSources.srcSourceDisabled',
  'sources.source_exists': 'errSources.srcSourceExists',
  'sources.dialog_unavailable': 'errSources.srcDialogUnavailable',
  'sources.release_not_found': 'errSources.srcReleaseNotFound',
  'sources.no_uri': 'errSources.srcNoUri',
  'sources.catalog_unavailable': 'errSources.srcCatalogUnavailable',
  'sources.feed_invalid_json': 'errSources.srcFeedInvalidJson',
  'sources.feed_unsupported_version': 'errSources.srcFeedUnsupportedVersion',
  'sources.feed_empty': 'errSources.srcFeedEmpty',
  'sources.feed_path_empty': 'errSources.srcFeedPathEmpty',
  'sources.feed_path_relative': 'errSources.srcFeedPathRelative',
  'sources.feed_not_regular_file': 'errSources.srcFeedNotRegularFile',
  'sources.feed_bad_scheme': 'errSources.srcFeedBadScheme',
  'sources.feed_too_large': 'errSources.srcFeedTooLarge',
  'sources.feed_bad_content_type': 'errSources.srcFeedBadContentType',
  'sources.feed_blocked_address': 'errSources.srcFeedBlockedAddress',
  'sources.feed_no_address': 'errSources.srcFeedNoAddress',
  'sources.feed_too_many_hops': 'errSources.srcFeedTooManyHops',
  'sources.feed_no_host': 'errSources.srcFeedNoHost',
  'sources.feed_invalid_url': 'errSources.srcFeedInvalidUrl',
  'sources.feed_bad_status': 'errSources.srcFeedBadStatus',
  'sources.feed_challenge': 'errSources.srcFeedChallenge',

  'discovery.busy': 'errSources.discoveryBusy',
  'discovery.not_started': 'errSources.discoveryNotStarted',
  'discovery.no_scan': 'errSources.discoveryNoScan',

  'lan.sharing_disabled': 'errSources.lanSharingDisabled',
  'lan.empty_game_id': 'errSources.lanEmptyGameId',
  'lan.library_unavailable': 'errSources.lanLibraryUnavailable',
  'lan.no_install_dir': 'errSources.lanNoInstallDir',
  'lan.exe_outside_install': 'errSources.lanExeOutsideInstall',
  'lan.games_path_unavailable': 'errSources.lanGamesPathUnavailable',
  'lan.offer_not_found': 'errSources.lanOfferNotFound',
  'lan.unknown_transfer': 'errSources.lanUnknownTransfer',
  'lan.invalid_info_hash': 'errSources.lanInvalidInfoHash',
  'lan.invalid_peer_id': 'errSources.lanInvalidPeerId',
  'lan.share_root_empty': 'errSources.lanShareRootEmpty',
  'lan.share_root_not_dir': 'errSources.lanShareRootNotDir',
  'lan.share_symlink': 'errSources.lanShareSymlink',
  'lan.share_irregular_file': 'errSources.lanShareIrregularFile',
};

export function sourceErrorText(err: unknown, fallback: string = msg('errSources.fallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}
