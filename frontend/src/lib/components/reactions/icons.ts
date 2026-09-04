import type { Component } from 'svelte';
import type { ReactionId } from '../../social/feed';
import Clap from './Clap.svelte';
import Eyes from './Eyes.svelte';
import Fire from './Fire.svelte';
import Heart from './Heart.svelte';
import Joy from './Joy.svelte';
import Party from './Party.svelte';
import Salute from './Salute.svelte';
import Skull from './Skull.svelte';

export const reactionIcons: Record<ReactionId, Component> = {
  fire: Fire,
  salute: Salute,
  heart: Heart,
  clap: Clap,
  skull: Skull,
  party: Party,
  eyes: Eyes,
  joy: Joy,
};
