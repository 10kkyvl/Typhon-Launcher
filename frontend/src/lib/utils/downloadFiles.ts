import type { FileState } from '../services/downloads';

export function withFiles<T extends { files?: FileState[] | null }>(item: T): T & { files: FileState[] } {
  return { ...item, files: item.files ?? [] };
}
