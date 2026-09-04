import type { Message } from '../../types';
import type { InstallKey } from '../ru/install';

export const install: Record<InstallKey, Message> = {
  'release.duplicateSources': { one: 'source', other: 'sources' },
  'verify.missingFiles': { one: '{count} file', other: '{count} files' },
  'verify.corruptedBlocks': { one: '{count} block', other: '{count} blocks' },
  'verify.unreadableFiles': { one: '{count} file', other: '{count} files' },
};
