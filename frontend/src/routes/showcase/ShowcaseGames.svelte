<script lang="ts">
  import { BellOff } from '@lucide/svelte';
  import GameListRow from '../../lib/components/GameListRow.svelte';
  import GameSavesCloud from '../../lib/components/GameSavesCloud.svelte';
  import HeroCarousel, { type HeroSlide } from '../../lib/components/HeroCarousel.svelte';
  import SaveConflictNotice from '../../lib/components/SaveConflictNotice.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { toast } from '../../lib/stores/toasts';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const LAUNCH_MS = 2000;

  // One sample game whose Play launches for two seconds, then runs until Stop: in the row and the carousel.
  let running = $state(false);
  let launching = $state(false);

  const said = (what: string) => (id: string) => toast(`${what}: ${id}`, 'info');

  // A row's own action that runs for two seconds, as Unwatch waits on Go.
  let unwatching = $state(false);
  function unwatch() {
    if (unwatching) return;
    unwatching = true;
    setTimeout(() => (unwatching = false), LAUNCH_MS);
  }

  function launch() {
    launching = true;
    setTimeout(() => {
      launching = false;
      running = true;
    }, LAUNCH_MS);
  }

  // The saves view's cloud copy whose Sync now runs for two seconds.
  let syncing = $state(false);
  function sync() {
    if (syncing) return;
    syncing = true;
    setTimeout(() => (syncing = false), LAUNCH_MS);
  }

  const pressed = (what: string) => () => toast(`${what} pressed`, 'info');
  const cloudActions = {
    onsettings: pressed('Open settings'),
    onsync: pressed('Sync now'),
    onresolve: pressed('Resolve'),
    onpull: pressed('Pull'),
    ondelete: pressed('Delete remote'),
  };
  const syncedAt = '2026-11-01T10:00:00Z';
  const conflict = { localUpdatedAt: '2026-11-01T10:00:00Z', remoteUpdatedAt: '2026-11-02T18:30:00Z', remoteDevice: 'Steam Deck' };

  // No art, a poster (the app's own picture), and a hero and a cover that both fail to load.
  const slides = $derived<HeroSlide[]>([
    { id: 'sample-celeste', title: 'Celeste', art: {}, line: 'No picture: its generated art. Press Play: it launches for two seconds, then runs', running, launching },
    { id: 'sample-poster', title: 'A game whose hero is a poster', art: { hero: '/typhon.png', heroIsPoster: true }, line: 'Played yesterday · 42 h' },
    { id: 'sample-broken', title: 'Hollow Knight', art: { hero: '/missing-hero.png', cover: '/missing-cover.png' }, line: 'Both pictures fail: the generated art' },
  ]);
</script>

<ShowcaseSection title="Games" note="The parts a game is shown with: the hero carousel, the list row, the cloud copy of its saves and its save conflict banner. Data and callbacks come from the page.">
  <ShowcaseRow caption="Hero carousel: three slides (no art, a poster, pictures that fail); moves every 7 s unless held" stack>
    <HeroCarousel level={4} {slides} onopen={said('Open')} onplay={launch} onstop={() => (running = false)} />
  </ShowcaseRow>

  <ShowcaseRow caption="Hero carousel: one slide, running; one slide, launching" stack>
    <HeroCarousel level={4} slides={[{ id: 'sample-outer-wilds', title: 'Outer Wilds', art: {}, line: 'Playing now', running: true }]} onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} />
    <HeroCarousel level={4} slides={[{ id: 'sample-hades', title: 'Hades', art: {}, line: 'Starting…', launching: true }]} onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} />
  </ShowcaseRow>

  <ShowcaseRow caption="List rows: idle with marks, one that launches when pressed, running, launching, favourite with no art" stack>
    <ul class="list">
      <GameListRow id="sample-hades" title="Hades" details={['Played yesterday', '42 h']} size="14.2 GB" onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')}>
        {#snippet marks()}<StatusBadge kind="neutral" label="Steam" dot={false} />{/snippet}
      </GameListRow>
      <GameListRow id="sample-celeste" title="Celeste" details={['Press Play: it launches for two seconds, then runs']} size="1.2 GB" {running} {launching} onopen={said('Open')} onplay={launch} onstop={() => (running = false)} onmore={said('More')} />
      <GameListRow id="sample-outer-wilds" title="Outer Wilds" details={['Playing now']} running onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')} />
      <GameListRow id="sample-tunic" title="Tunic" details={['Starting…']} size="2.8 GB" launching onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')} />
      <GameListRow id="sample-disco" title="Disco Elysium – The Final Cut, a title long enough to be cut" details={['Never played']} size="21 GB" favorite onopen={said('Open')} onmore={said('More')} />
    </ul>
  </ShowcaseRow>

  <ShowcaseRow caption="List rows as the library draws them: plain titles, the backdrop follows the row, a watched game with Unwatch (busy for two seconds when pressed)" stack>
    <ul class="list">
      <GameListRow id="sample-hades" title="Hades" details={['Played yesterday', '42 h']} size="14.2 GB" titleTone="plain" ambient onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')} />
      <GameListRow
        id="sample-silksong"
        title="Hollow Knight: Silksong"
        details={['Watched']}
        titleTone="plain"
        ambient
        actions={[{ id: 'unwatch', label: 'Unwatch', icon: BellOff, busy: unwatching, onclick: unwatch }]}
        onopen={said('Open')}
      />
    </ul>
  </ShowcaseRow>

  <ShowcaseRow caption="Saves view, the cloud copy: off, signed out, unknown to the catalog, no copy yet, synced (Sync now runs for two seconds), held back while the game runs" stack>
    <div class="part"><GameSavesCloud availability="off" {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="signed-out" {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="no-catalog" {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="on" {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="on" game={{ state: 'synced', code: '', syncedAt }} busy={syncing} {...cloudActions} onsync={sync} /></div>
    <div class="part"><GameSavesCloud availability="on" game={{ state: 'synced', code: '', syncedAt }} running {...cloudActions} /></div>
  </ShowcaseRow>

  <ShowcaseRow caption="Saves view, the cloud copy: in conflict, failed with its code's text, left out by the player" stack>
    <div class="part"><GameSavesCloud availability="on" game={{ state: 'conflict', code: '', syncedAt }} {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="on" game={{ state: 'failed', code: 'cloud.network', syncedAt }} {...cloudActions} /></div>
    <div class="part"><GameSavesCloud availability="on" game={{ state: 'skipped', code: 'cloud.excluded', syncedAt: '' }} {...cloudActions} /></div>
  </ShowcaseRow>

  <ShowcaseRow caption="Save conflict banner of the game page: both dates and the cloud copy's device, without a device, before the details load" stack>
    <SaveConflictNotice {conflict} onresolve={pressed('Resolve conflict')} />
    <SaveConflictNotice conflict={{ ...conflict, remoteDevice: '' }} onresolve={pressed('Resolve conflict')} />
    <SaveConflictNotice onresolve={pressed('Resolve conflict')} />
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .part {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 64rem;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-width: 96rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
