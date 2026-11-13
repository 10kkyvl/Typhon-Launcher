<script lang="ts">
  import GameListRow from '../../lib/components/GameListRow.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { toast } from '../../lib/stores/toasts';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const LAUNCH_MS = 2000;

  let running = $state(false);
  let launching = $state(false);

  const said = (what: string) => (id: string) => toast(`${what}: ${id}`, 'info');

  function launch() {
    launching = true;
    setTimeout(() => {
      launching = false;
      running = true;
    }, LAUNCH_MS);
  }
</script>

<ShowcaseSection title="Games" note="The parts a game is shown with: the list row. Data and callbacks come from the page.">
  <ShowcaseRow caption="List rows: idle with marks, launching, running, favourite with no art" stack>
    <ul class="list">
      <GameListRow id="sample-hades" title="Hades" details={['Played yesterday', '42 h']} size="14.2 GB" onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')}>
        {#snippet marks()}<StatusBadge kind="neutral" label="Steam" dot={false} />{/snippet}
      </GameListRow>
      <GameListRow id="sample-celeste" title="Celeste" details={['Press Play: it launches for two seconds, then runs']} size="1.2 GB" {running} {launching} onopen={said('Open')} onplay={launch} onstop={() => (running = false)} onmore={said('More')} />
      <GameListRow id="sample-outer-wilds" title="Outer Wilds" details={['Playing now']} running onopen={said('Open')} onplay={said('Play')} onstop={said('Stop')} onmore={said('More')} />
      <GameListRow id="sample-disco" title="Disco Elysium – The Final Cut, a title long enough to be cut" details={['Never played']} size="21 GB" favorite onopen={said('Open')} onmore={said('More')} />
    </ul>
  </ShowcaseRow>
</ShowcaseSection>

<style>
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
