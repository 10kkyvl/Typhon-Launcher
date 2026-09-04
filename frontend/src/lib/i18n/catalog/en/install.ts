import type { Message } from '../../types';
import type { InstallKey } from '../ru/install';

export const install: Record<InstallKey, Message> = {
  'release.duplicateSources': { one: 'source', other: 'sources' },
  'release.loading': 'Loading releases…',
  'release.new': 'New',
  'release.unavailable': 'Unavailable',
  'verify.missingFiles': { one: '{count} file', other: '{count} files' },
  'verify.corruptedBlocks': { one: '{count} block', other: '{count} blocks' },
  'verify.unreadableFiles': { one: '{count} file', other: '{count} files' },
  'verify.fileIntegrity': 'File integrity',
  'verify.unavailable': 'Check unavailable',
  'verify.neverChecked': 'Never checked',
  'verify.damageFound': 'Damage found',
  'verify.notFullyChecked': 'Not fully checked',
  'verify.filesOk': 'Files are fine',
  'verify.unavailableExplain':
    "Nothing to compare against: the release doesn't provide a file list suitable for verification, and no manifest has been recorded yet. Record a manifest of the current files — later checks will show what changed.",
  'verify.createManifest': 'Create manifest',
  'verify.gameRunningManifest': 'The game is running. Close it to record the manifest.',
  'verify.matched': 'Matched',
  'verify.checked': 'Checked',
  'verify.missing': 'Missing',
  'verify.corrupted': 'Corrupted',
  'verify.unreadable': 'Unreadable',
  'verify.byTorrent': 'via the release torrent',
  'verify.byManifest': 'via the saved manifest',
  'verify.summary': 'Checked {method}, {date}.',
  'verify.someFilesLocked':
    "Some files were locked by another program — this isn't corruption, retry the check later.",
  'verify.explain':
    'The check compares files on disk against the release torrent or the saved manifest. The result applies to the current version and install folder.',
  'verify.verifyFiles': 'Verify files',
  'verify.repair': 'Repair',
  'verify.gameRunningVerify': 'The game is running. Close it before verifying files.',
};
