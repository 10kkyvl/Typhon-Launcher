<script lang="ts">
  import { ArrowLeft, Bell, ChevronDown, ExternalLink, Heart, Pause, Play, Plus, Settings, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import CountBadge from '../../lib/components/CountBadge.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { toast } from '../../lib/stores/toasts';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const VARIANTS = ['primary', 'secondary', 'ghost', 'danger'] as const;
  const SIZES = ['sm', 'md', 'lg'] as const;
  const BUSY_MS = 1500;

  let following = $state(false);
  let muted = $state(true);
  let busy = $state(false);
  let chipBusy = $state(false);
  let filters = $state<Record<string, boolean>>({ Action: true, Puzzle: false, Racing: false });
  let tags = $state(['Co-op', 'Controller support', 'A tag long enough to show how a chip holds a long name']);

  const wait = () => new Promise<void>((resolve) => setTimeout(resolve, BUSY_MS));

  async function run() {
    busy = true;
    await wait();
    busy = false;
    toast('The action finished', 'success');
  }

  async function vote() {
    chipBusy = true;
    await wait();
    chipBusy = false;
  }

  const pressed = (what: string) => () => toast(`${what} pressed`, 'info');
</script>

<ShowcaseSection title="Actions" note="Buttons, icon buttons, chips and text links. Hover, focus with Tab and press them: each one acts.">
  <ShowcaseRow caption="Button variants">
    {#each VARIANTS as variant (variant)}
      <Button {variant} onclick={pressed(variant)}>{variant}</Button>
    {/each}
  </ShowcaseRow>

  <ShowcaseRow caption="Button sizes">
    {#each SIZES as size (size)}
      <Button variant="primary" {size} onclick={pressed(`Size ${size}`)}>Size {size}</Button>
    {/each}
    <Button size="sm" icon aria-label="Add" onclick={pressed('Add')}><Plus size="1.4rem" /></Button>
    <Button icon aria-label="Settings" onclick={pressed('Settings')}><Settings size="1.6rem" /></Button>
  </ShowcaseRow>

  <ShowcaseRow caption="Button states">
    <Button disabled>Disabled</Button>
    <Button variant="primary" disabled reason="Sign in to sync your saves">Disabled with a reason</Button>
    <Button variant="primary" {busy} onclick={run}>Run for a moment</Button>
    <Button busy>Busy</Button>
    <Button pressed={following} onclick={() => (following = !following)}>Follow</Button>
    <Button minWidth="16rem" onclick={pressed('Wide')}>Keeps its width</Button>
  </ShowcaseRow>

  <ShowcaseRow caption="Joined, block, a long label and a count">
    <span class="joined">
      <Button variant="primary" join="end" onclick={pressed('Play')}><Play size="1.4rem" />Play</Button>
      <Button variant="primary" join="start" icon aria-label="Play profile" onclick={pressed('Profile')}><ChevronDown size="1.4rem" /></Button>
    </span>
    <Button onclick={pressed('Long')}>A button whose label runs on for longer than any label should</Button>
    <Button onclick={pressed('Friends')}>Friends <CountBadge count={3} label="3 friend requests" /></Button>
    <Button variant="primary" onclick={pressed('Inbox')}>Inbox <CountBadge count={120} label="120 unread messages" on="accent" /></Button>
    <Button onclick={pressed('Empty')}>Nothing to count <CountBadge count={0} label="no requests" /></Button>
    <span class="block"><Button block onclick={pressed('Block')}>Fills its row</Button></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Icon buttons">
    <IconButton label="Notifications" onclick={pressed('Ghost')}><Bell size="1.8rem" /></IconButton>
    <IconButton label="Notifications" variant="secondary" onclick={pressed('Secondary')}><Bell size="1.8rem" /></IconButton>
    <IconButton label="New chat" variant="primary" onclick={pressed('Primary')}><Plus size="1.8rem" /></IconButton>
    <IconButton label="Small" size="sm" onclick={pressed('Small')}><Settings size="1.5rem" /></IconButton>
    <IconButton label="Large" size="lg" onclick={pressed('Large')}><Settings size="2rem" /></IconButton>
    <IconButton label="Mute" pressed={muted} onclick={() => (muted = !muted)}><Bell size="1.8rem" /></IconButton>
    <IconButton label="Remove from favourites" lit onclick={pressed('Favourite')}><Heart size="1.8rem" /></IconButton>
    <IconButton label="Back" disabled reason="Nothing to go back to"><ArrowLeft size="1.8rem" /></IconButton>
    <IconButton label="Pause" busy><Pause size="1.8rem" /></IconButton>
    <IconButton label="Attach" hint="Images up to 10 MB" onclick={pressed('Attach')}><Plus size="1.8rem" /></IconButton>
    <IconButton label="Friends" count={1} countLabel="Friends, 1 request" onclick={pressed('One')}><Users size="1.8rem" /></IconButton>
    <IconButton label="Friends" count={120} countLabel="Friends, 120 requests" variant="primary" onclick={pressed('Many')}><Users size="1.8rem" /></IconButton>
  </ShowcaseRow>

  <ShowcaseRow caption="Chips">
    {#each Object.keys(filters) as name (name)}
      <Chip selected={filters[name]} onclick={() => (filters[name] = !filters[name])}>{name}</Chip>
    {/each}
    <Chip variant="filled" onclick={pressed('Filled')}>Filled</Chip>
    <Chip variant="quiet" onclick={pressed('Quiet')}>Quiet</Chip>
    <Chip hint="Shows the games friends play" onclick={pressed('Hinted')}>With a hint</Chip>
    <Chip disabled reason="Only for installed games" onclick={pressed('Refused')}>Disabled with a reason</Chip>
    <Chip busy={chipBusy} onclick={vote}>Vote</Chip>
    <Chip href="#showcase-top">
      A link chip
      {#snippet end()}<ExternalLink />{/snippet}
    </Chip>
  </ShowcaseRow>

  <ShowcaseRow caption="Removable chips">
    {#each tags as tag (tag)}
      <Chip onremove={() => (tags = tags.filter((item) => item !== tag))} removeLabel={`Remove ${tag}`}>{tag}</Chip>
    {/each}
    {#if tags.length === 0}
      <TextLink onclick={() => (tags = ['Co-op', 'Controller support'])}>Bring the tags back</TextLink>
    {/if}
  </ShowcaseRow>

  <ShowcaseRow caption="Text links in running text" stack>
    <p class="prose">
      A link in running text <TextLink href="#showcase-top">goes somewhere and wraps across lines with the sentence it sits in</TextLink>,
      while an action <TextLink onclick={pressed('Accent action')}>does something here</TextLink> and keeps to one box.
    </p>
    <p class="prose">
      Muted links are furniture: <TextLink tone="muted" href="#showcase-top">a muted link</TextLink> and
      <TextLink tone="muted" onclick={pressed('Muted action')}>a muted action</TextLink>; a disabled action
      <TextLink onclick={pressed('Disabled')} disabled>cannot be pressed</TextLink>.
    </p>
    <p class="prose">
      <TextLink tone="muted" onclick={pressed('Back')}>
        {#snippet start()}<ArrowLeft />{/snippet}
        Back to the library
      </TextLink>
      ·
      <TextLink href="#showcase-top">
        Open the wiki
        {#snippet end()}<ExternalLink />{/snippet}
      </TextLink>
    </p>
  </ShowcaseRow>

  <ShowcaseRow caption="Text links kept to one line" stack>
    <p class="prose narrow">
      <TextLink href="#showcase-top" truncate>The Legend of Heroes: Trails in the Sky the 3rd, a title too long for its row</TextLink>
    </p>
    <p class="prose narrow">
      <TextLink onclick={pressed('One-line action')} truncate>An action whose label is cut where its row ends</TextLink>
    </p>
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .joined {
    display: inline-flex;
  }

  .block {
    display: flex;
    flex: 1 1 100%;
  }

  .prose {
    max-width: var(--prose-max);
    font-size: var(--font-md);
    color: var(--text-2);
  }

  .narrow {
    max-width: 24rem;
  }
</style>
