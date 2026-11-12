<script lang="ts">
  import { Clock, EyeOff, Gamepad2, Inbox, Trophy } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import CountBadge from '../../lib/components/CountBadge.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import ProgressBar, { type ProgressTone } from '../../lib/components/ProgressBar.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { toast } from '../../lib/stores/toasts';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const KINDS = ['success', 'warning', 'danger', 'accent', 'neutral'] as const;
  const SPINNER_SIZES = ['sm', 'md', 'lg'] as const;
  const TONES: ProgressTone[] = ['accent', 'success', 'warning', 'danger', 'muted'];
  const COUNTS = [0, 1, 7, 99, 120];

  let progress = $state(40);
</script>

<ShowcaseSection title="Status" note="Badges, counts, spinners, progress, tiles, cards and empty states.">
  <ShowcaseRow caption="Status badges">
    {#each KINDS as kind (kind)}
      <StatusBadge {kind} label={kind} />
    {/each}
  </ShowcaseRow>

  <ShowcaseRow caption="Plain, without a dot, with an icon, with a hint">
    {#each KINDS as kind (kind)}
      <StatusBadge {kind} label={kind} plain />
    {/each}
    <StatusBadge label="No dot" dot={false} />
    <StatusBadge label="Hidden" kind="neutral">
      {#snippet icon()}<EyeOff />{/snippet}
    </StatusBadge>
    <StatusBadge label="Outdated" kind="warning" hint="A newer release of this game is out" />
  </ShowcaseRow>

  <ShowcaseRow caption="A long label that wraps" stack>
    <span class="narrow">
      <StatusBadge kind="accent" label="The Elder Scrolls V: Skyrim Special Edition – Anniversary Upgrade" wrap />
    </span>
  </ShowcaseRow>

  <ShowcaseRow caption="Count badges: 0 shows nothing, more than 99 is 99+">
    {#each COUNTS as count (count)}
      <Button onclick={() => toast(`${count} counted`, 'info')}>Count {count} <CountBadge {count} label={`${count} new`} /></Button>
    {/each}
  </ShowcaseRow>

  <ShowcaseRow caption="Spinners">
    {#each SPINNER_SIZES as size (size)}
      <Spinner {size} />
    {/each}
    <span class="on-accent"><Spinner tone="current" /></span>
    <Spinner size="sm" label="Loading the list" />
  </ShowcaseRow>

  <ShowcaseRow caption="Progress bars" stack>
    {#each TONES as tone (tone)}
      <ProgressBar value={progress} {tone} label={`Download, ${tone} tone`} />
    {/each}
    <ProgressBar value={0} indeterminate label="Waiting for the server" />
    <ProgressBar value={3} max={12} height={8} label="Files checked" valueText="3 of 12 files" />
    <span class="controls">
      <Button size="sm" onclick={() => (progress = Math.max(0, progress - 10))}>Less</Button>
      <Button size="sm" onclick={() => (progress = Math.min(100, progress + 10))}>More</Button>
    </span>
  </ShowcaseRow>

  <ShowcaseRow caption="Stat tiles">
    <StatTile value="128 h" label="Played">
      {#snippet icon()}<Clock />{/snippet}
    </StatTile>
    <StatTile value={42} label="Games">
      {#snippet icon()}<Gamepad2 />{/snippet}
    </StatTile>
    <StatTile value="7 / 50" label="Achievements" />
  </ShowcaseRow>

  <ShowcaseRow caption="Cards">
    <Card title="A card with an action">
      {#snippet action()}<TextLink onclick={() => toast('Show all pressed', 'info')}>Show all</TextLink>{/snippet}
      <p class="body">Medium padding, the card surface.</p>
    </Card>
    <Card title="A panel" surface="panel" padding="sm">
      <p class="body">Small padding, the panel surface.</p>
    </Card>
    <Card padding="md-even">
      <p class="body">A card with no title and even padding.</p>
    </Card>
  </ShowcaseRow>

  <ShowcaseRow caption="Empty states: page, centred, inline" stack>
    <EmptyState title="No games yet" description="Add a folder with games or download one from the catalogue." level={4}>
      {#snippet icon()}<Gamepad2 size="3.2rem" />{/snippet}
      {#snippet actions()}<Button variant="primary" onclick={() => toast('Add pressed', 'info')}>Add a folder</Button>{/snippet}
    </EmptyState>
    <EmptyState title="No achievements" description="This game has none to earn." align="center" level={4}>
      {#snippet icon()}<Trophy size="3.2rem" />{/snippet}
    </EmptyState>
    <EmptyState size="inline" title="Nothing in this list" description="Inline: a quiet note with no heading." />
    <EmptyState size="inline" lead title="No saves found" description="Lead: the title is body text, the description its hint.">
      {#snippet icon()}<Inbox size="2rem" />{/snippet}
    </EmptyState>
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .narrow {
    display: flex;
    max-width: 24rem;
  }

  .on-accent {
    display: inline-flex;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--accent-on);
  }

  .controls {
    display: flex;
    gap: var(--space-2);
  }

  .body {
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
