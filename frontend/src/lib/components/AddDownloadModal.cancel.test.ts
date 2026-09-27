import { describe, expect, it } from 'vitest';
import source from './AddDownloadModal.svelte?raw';

// Reproduces the real user report: closing the add-download window while
// FetchMetadata is still waiting for a magnet's metadata used to leave the
// Go call running for up to 90s with the torrent's infohash reserved, so an
// immediate retry was rejected as "already used by another download" even
// though nothing else was using it. reset() runs on every close (the $effect
// depends on `open`), so it is the one place that can tell Go to give up on
// a fetch that is still in flight.
describe('AddDownloadModal metadata fetch cancellation', () => {
  it('imports cancelFetchMetadata from the downloads service', () => {
    expect(source).toContain('cancelFetchMetadata');
    expect(source).toMatch(/import \{[^}]*cancelFetchMetadata[^}]*\} from '\.\.\/services\/downloads';/);
  });

  it('cancels the in-flight fetch in reset() while still on the loading step', () => {
    expect(source).toMatch(
      /function reset\(\) \{\s*token\+\+;\s*if \(step === 'loading'\) \{\s*cancelFetchMetadata\(source\);\s*\}/,
    );
  });
});
