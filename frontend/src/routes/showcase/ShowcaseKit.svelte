<script lang="ts">
  import DiskMeter from '../../lib/components/DiskMeter.svelte';
  import { gameTone, type MeterSegment } from '../../lib/components/diskMeter';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const GB = 1024 ** 3;
  const TOTAL = 1000 * GB;

  const DISK: MeterSegment[] = [
    { key: 'games', label: 'Games', bytes: 420 * GB, tone: 'game' },
    { key: 'downloads', label: 'Download data', bytes: 85 * GB, tone: 'data' },
    { key: 'leftovers', label: 'Leftovers', bytes: 12 * GB, tone: 'reclaim' },
    { key: 'other', label: 'Other files', bytes: 210 * GB, tone: 'other' },
  ];
  const GAMES: MeterSegment[] = ['Elden Ring', 'Cyberpunk 2077', 'Hades II', 'Baldur’s Gate 3', 'Celeste', 'Hollow Knight'].map((label, index) => ({
    key: label,
    label,
    bytes: [60, 70, 15, 120, 1, 9][index] * GB,
    tone: gameTone(index),
  }));
  const OVERFLOW: MeterSegment[] = [
    { key: 'games', label: 'Games', bytes: 800 * GB, tone: 'game' },
    { key: 'other', label: 'Other files', bytes: 400 * GB, tone: 'other' },
  ];
  const TINY: MeterSegment[] = [
    { key: 'big', label: 'One big game', bytes: 999 * GB, tone: 'game' },
    { key: 'tiny', label: 'A save of 4 KB', bytes: 4096, tone: 'game-2' },
  ];
  const UNMEASURED: MeterSegment[] = [
    { key: 'games', label: 'Games', bytes: 300 * GB, tone: 'game', sizeUnknown: true },
    { key: 'screens', label: 'Screenshots', bytes: 0, tone: 'data', sizeUnknown: true },
  ];

  let picked = $state('none yet');
</script>

<ShowcaseSection title="Screen parts" note="The disk meter, the settings row and the bar chart the screens share.">
  <ShowcaseRow caption="Disk meter: a drive, every tone but the game steps" stack>
    <DiskMeter label="Drive C:" segments={DISK} total={TOTAL} free={273 * GB} />
  </ShowcaseRow>

  <ShowcaseRow caption="Game segments to pick: the three steps in turn, arrows between them, a hint on each" stack>
    <DiskMeter label="Library drive" segments={GAMES} total={TOTAL} onpick={(key) => (picked = key)} />
    <span class="picked">Picked: {picked}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Overflow: the segments sum over the total, no free space is drawn" stack>
    <DiskMeter label="Drive D:" segments={OVERFLOW} total={TOTAL} />
  </ShowcaseRow>

  <ShowcaseRow caption="Unknown total, and sizes not measured (a lower bound, an unknown)" stack>
    <DiskMeter label="Network drive" segments={UNMEASURED} total={0} />
    <DiskMeter label="Drive E:" segments={UNMEASURED} total={TOTAL} free={null} />
  </ShowcaseRow>

  <ShowcaseRow caption="A segment far under a pixel still shows as a hairline" stack>
    <DiskMeter label="Drive F:" segments={TINY} total={TOTAL} />
  </ShowcaseRow>

  <ShowcaseRow caption="Compact, for a settings row: the bar and one line" stack>
    <div class="compact">
      <DiskMeter label="Library folder" segments={DISK.slice(0, 1)} total={TOTAL} free={273 * GB} compact />
    </div>
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .picked {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .compact {
    max-width: 32rem;
  }
</style>
