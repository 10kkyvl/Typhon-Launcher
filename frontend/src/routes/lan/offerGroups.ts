import type { Offer } from '../../lib/services/lan';

/** The offers of one computer nearby, as Go knows it: by its id, named by its host. */
export interface OfferGroup {
  peerId: string;
  host: string;
  offers: Offer[];
}

/**
 * The offers by the computer that makes them, each computer where its first offer stood. Computers are
 * told apart by their id: two that share a host name are two groups.
 */
export function offersByComputer(list: readonly Offer[]): OfferGroup[] {
  const groups = new Map<string, OfferGroup>();
  for (const offer of list) {
    const group = groups.get(offer.peerId);
    if (group) group.offers.push(offer);
    else groups.set(offer.peerId, { peerId: offer.peerId, host: offer.host, offers: [offer] });
  }
  return [...groups.values()];
}

/** The key of an offer's row: Go keys an offer by its computer and its infohash. */
export const offerKey = (offer: Pick<Offer, 'peerId' | 'infoHash'>) => `${offer.peerId}:${offer.infoHash}`;
