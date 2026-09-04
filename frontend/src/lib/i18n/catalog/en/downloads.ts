import type { Message } from '../../types';
import type { DownloadsKey } from '../ru/downloads';

export const downloads: Record<DownloadsKey, Message> = {
  'downloads.selected': { one: 'Selected: {count} file, {size}', other: 'Selected: {count} files, {size}' },
  'downloads.active': { one: '{count} active', other: '{count} active' },
};
