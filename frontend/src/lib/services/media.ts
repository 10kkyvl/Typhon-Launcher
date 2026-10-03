import { Service as MediaService } from '../../../bindings/typhon/internal/media';
import { inWails } from './backend';

export interface MediaTrack {
  app: string;
  title: string;
  artist: string;
  album: string;
  playing: boolean;
  canPlayPause: boolean;
  canNext: boolean;
  canPrev: boolean;
}

export interface MediaState {
  supported: boolean;
  active: boolean;
  track: MediaTrack;
}

const emptyTrack: MediaTrack = {
  app: '',
  title: '',
  artist: '',
  album: '',
  playing: false,
  canPlayPause: false,
  canNext: false,
  canPrev: false,
};

export function toMediaState(value: unknown): MediaState {
  const state = value as Partial<MediaState> | null;
  const track = (state?.track ?? {}) as Partial<MediaTrack>;
  return {
    supported: state?.supported === true,
    active: state?.active === true,
    track: {
      app: track.app ?? '',
      title: track.title ?? '',
      artist: track.artist ?? '',
      album: track.album ?? '',
      playing: track.playing === true,
      canPlayPause: track.canPlayPause === true,
      canNext: track.canNext === true,
      canPrev: track.canPrev === true,
    },
  };
}

export async function currentMedia(): Promise<MediaState> {
  if (!inWails) return { supported: false, active: false, track: emptyTrack };
  return toMediaState(await MediaService.Current());
}

export async function mediaToggle(): Promise<void> {
  await MediaService.TogglePlayPause();
}

export async function mediaNext(): Promise<void> {
  await MediaService.Next();
}

export async function mediaPrevious(): Promise<void> {
  await MediaService.Previous();
}
