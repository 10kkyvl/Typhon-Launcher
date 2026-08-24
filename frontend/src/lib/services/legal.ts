import { Service as LegalService } from '../../../bindings/typhon/internal/legal';
import type { Document as RawDocument, Meta as RawMeta } from '../../../bindings/typhon/internal/legal/models';
import { inWails } from './backend';

export interface LegalMeta {
  id: string;
  title: string;
}

export interface LegalDocument {
  id: string;
  title: string;
  body: string;
}

const unavailable = () => new Error('unavailable in browser');

function toMeta(raw: RawMeta): LegalMeta {
  return { id: raw.ID, title: raw.Title };
}

function toDocument(raw: RawDocument): LegalDocument {
  return { id: raw.ID, title: raw.Title, body: raw.Body };
}

export async function listLegalDocuments(): Promise<LegalMeta[]> {
  if (!inWails) throw unavailable();
  const raw = (await LegalService.List()) ?? [];
  return raw.map(toMeta);
}

export async function getLegalDocument(id: string): Promise<LegalDocument> {
  if (!inWails) throw unavailable();
  const raw = await LegalService.Get(id);
  return toDocument(raw);
}
