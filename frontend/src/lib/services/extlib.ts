import { Service as ExtlibService } from '../../../bindings/typhon/internal/extlib';
import { inWails } from './backend';

export interface ExtlibSkipped {
  title: string;
  reason: string;
}

export interface ExtlibProviderResult {
  provider: string;
  found: number;
  added: number;
  updated: number;
  skipped: ExtlibSkipped[] | null;
  notInstalled: boolean;
  error: string;
}

export interface ExtlibSummary {
  providers: ExtlibProviderResult[] | null;
  added: number;
  updated: number;
  metadataErrors: string[] | null;
}

const unavailable = () => new Error('unavailable in browser');

export async function importExternalGames(): Promise<ExtlibSummary> {
  if (!inWails) throw unavailable();
  return (await ExtlibService.Import()) as unknown as ExtlibSummary;
}
