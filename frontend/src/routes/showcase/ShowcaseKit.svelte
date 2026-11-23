<script lang="ts">
  import BarChart from '../../lib/components/BarChart.svelte';
  import { secondsScale, type ChartBucket } from '../../lib/components/barChart';
  import DiskMeter from '../../lib/components/DiskMeter.svelte';
  import { gameTone, type MeterSegment } from '../../lib/components/diskMeter';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Button from '../../lib/components/Button.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import ScheduleEditor from '../../lib/components/ScheduleEditor.svelte';
  import type { ScheduleWindow } from '../../lib/services/settings';
  import Select from '../../lib/components/Select.svelte';
  import type { SelectOption } from '../../lib/components/select';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import Slider from '../../lib/components/Slider.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { playtime } from '../../lib/utils/format';
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

  // More games than a compact bar draws at their size: the smallest fold into one part.
  const MANY: MeterSegment[] = Array.from({ length: 60 }, (_, index) => ({
    key: `game-${index}`,
    label: `Game ${index + 1}`,
    bytes: (4 + (index % 7) * 3) * GB,
    tone: gameTone(index),
  }));
  const UNDRAWN: MeterSegment[] = [
    ...GAMES.slice(0, 2),
    { key: 'new', label: 'Just installed, not measured yet', bytes: 0, tone: 'game-3', sizeUnknown: true },
    { key: 'empty', label: 'An empty folder', bytes: 0, tone: 'game' },
  ];

  // Three small games on a 2 TB drive: none takes a hairline of the bar, so the largest stays beside
  // the fold of the other two.
  const TB = 1024 * GB;
  const ALL_TINY: MeterSegment[] = [
    { key: 'celeste', label: 'Celeste', bytes: 1 * GB, tone: 'game' },
    { key: 'hades', label: 'Hades', bytes: 5 * GB, tone: 'game-2' },
    { key: 'inside', label: 'Inside', bytes: 3 * GB, tone: 'game-3' },
  ];
  const BOUND_GAME: MeterSegment[] = [{ key: 'games', label: 'Games (still being measured)', bytes: 420 * GB, tone: 'game', sizeUnknown: true }];
  const MEASURING: MeterSegment[] = [...GAMES.slice(0, 2), { key: 'measuring', label: 'Starfield (still being measured)', bytes: 40 * GB, tone: 'game-3', sizeUnknown: true }];
  // A hundred games on the full bar: the smallest fold, and the fold's entry in the legend opens.
  const HUNDRED: MeterSegment[] = Array.from({ length: 100 }, (_, index) => ({
    key: `lib-${index}`,
    label: `Game ${index + 1}`,
    bytes: (index < 40 ? 20 + (index % 5) * 6 : 0.2 + (index % 4) * 0.1) * GB,
    tone: gameTone(index),
  }));
  let tinyPicked = $state('none yet');
  let hundredPicked = $state('none yet');
  let why = $state(false);

  let picked = $state('none yet');
  let folded = $state('none yet');
  let undrawn = $state('none yet');

  const uid = $props.id();
  const LANGUAGES: SelectOption[] = [
    { id: 'en', label: 'English' },
    { id: 'ru', label: 'Russian' },
  ];
  let startup = $state(true);
  let motion = $state(true);
  let language = $state('en');
  let scale = $state(2);
  let threads = $state(4);
  const SCALES = ['80%', '90%', '100%', '110%', '125%'];

  const HOUR = 3600;
  const time = (seconds: number) => (seconds > 0 ? playtime(seconds) : 'No play');
  const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  const WEEK: ChartBucket[] = [1.5, 0, 3.25, 0.5, 6.75, 0, 0].map((hours, index) => ({
    key: DAY_NAMES[index],
    label: `${DAY_NAMES[index]}, ${index + 2} November`,
    tick: DAY_NAMES[index],
    value: hours * HOUR,
    current: index === 4,
  }));
  const WEEKS: ChartBucket[] = [4, 11, 0, 7.5, 2, 19, 9, 12].map((hours, index, all) => ({
    key: `w${index}`,
    label: index === all.length - 1 ? 'This week' : `Week of ${7 * index + 1 > 30 ? 7 * index - 29 : 7 * index + 1} ${index < 5 ? 'September' : 'October'}`,
    tick: index === all.length - 1 ? 'Now' : `W${index + 1}`,
    value: hours * HOUR,
    current: index === all.length - 1,
  }));
  const HUGE: ChartBucket[] = [0.2, 0.1, 40, 0.3, 0.02].map((hours, index) => ({ key: `d${index}`, label: `Day ${index + 1}`, value: hours * HOUR }));
  const NOTHING: ChartBucket[] = WEEK.map((day) => ({ ...day, value: 0 }));
  let week = $state('w7');
  let notYet = $state<string | null>(null);
  let stackOn = $state(true);
  let stackLanguage = $state('en');
  let stackScale = $state(2);
  let limit = $state(25 * 1024 * 1024);
  let route = $state('direct');
  let windows = $state<ScheduleWindow[]>([{ days: [1, 2, 3, 4, 5], from: '01:00', to: '07:00' }]);
  const ROUTES = [
    { id: 'direct', label: 'Direct' },
    { id: 'interface', label: 'Interface' },
    { id: 'proxy', label: 'Proxy' },
  ];
</script>

<ShowcaseSection title="Screen parts" note="The disk meter, the settings row and the bar chart the screens share.">
  <ShowcaseRow caption="Disk meter: a drive, every tone but the game steps" stack>
    <DiskMeter label="Drive C:" segments={DISK} total={TOTAL} free={273 * GB} />
  </ShowcaseRow>

  <ShowcaseRow caption="Game segments to pick: the three steps in turn, arrows between them, a hint on each" stack>
    <DiskMeter label="Library drive" segments={GAMES} total={TOTAL} onpick={(key) => (picked = key)} />
    <span class="picked">Picked: {picked}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Overflow: the segments sum over the total; drawn in proportion to each other, free space not measured so neither drawn nor said, the words say so" stack>
    <DiskMeter label="Drive D:" segments={OVERFLOW} total={TOTAL} />
  </ShowcaseRow>

  <ShowcaseRow caption="Overflow with a measured free space: 600 GB of games and 500 GB free on a 1,000 GB drive; the free space is said and drawn as the track the parts leave" stack>
    <div data-testid="kit-meter-overflow-free">
      <DiskMeter label="Drive H:" segments={GAMES.slice(0, 1).map((game) => ({ ...game, bytes: 600 * GB }))} total={TOTAL} free={500 * GB} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Every game too small to draw: three games on a 2 TB drive; the largest stays (a hairline) beside the fold of the other two, each pickable" stack>
    <div data-testid="kit-meter-tiny">
      <DiskMeter label="Drive T:" segments={ALL_TINY} total={2 * TB} free={1948 * GB} onpick={(key) => (tinyPicked = key)} foldLabel={(count) => `${count} more games`} />
    </div>
    <span class="picked">Picked: {tinyPicked}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="A game still being measured beside other files: its size a lower bound, the space beside it at most the gap, drawn filled" stack>
    <div data-testid="kit-meter-at-most">
      <DiskMeter label="Drive I:" segments={BOUND_GAME} total={TOTAL} free={500 * GB} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="A hundred games on the full bar: the fold's entry in the legend opens to list the folded games, each a pick" stack>
    <div data-testid="kit-meter-fold-open">
      <DiskMeter label="Library drive" segments={HUNDRED} total={2 * TB} free={600 * GB} onpick={(key) => (hundredPicked = key)} foldLabel={(count) => `${count} more games`} />
    </div>
    <span class="picked">Picked: {hundredPicked}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Space no segment accounts for, with the free space measured: drawn and listed as other files, not as free" stack>
    <DiskMeter label="Drive G:" segments={GAMES} total={TOTAL} free={600 * GB} />
  </ShowcaseRow>

  <ShowcaseRow caption="The free space not measured (free null), every game measured: the space after the games is outlined, not bare track, and said as other files and free space, exact; full and compact" stack>
    <div data-testid="kit-meter-free-null">
      <DiskMeter label="Drive J" segments={GAMES} total={TOTAL} free={null} />
    </div>
    <div class="compact" data-testid="kit-meter-free-null-compact">
      <DiskMeter label="Drive J" segments={GAMES} total={TOTAL} free={null} compact />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="The free space left out beside a game still being measured: it cannot be derived, so the space after the games is outlined and said as other files and free space, at most; full and compact" stack>
    <div data-testid="kit-meter-rest-bound">
      <DiskMeter label="Drive K" segments={MEASURING} total={TOTAL} />
    </div>
    <div class="compact" data-testid="kit-meter-rest-bound-compact">
      <DiskMeter label="Drive K" segments={MEASURING} total={TOTAL} compact />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Sixty games in a compact bar: the smallest fold into one part (a button here), the bar stays inside its track" stack>
    <div class="compact" data-testid="kit-meter-fold">
      <DiskMeter label="Library folder" segments={MANY} total={TOTAL} compact onpick={(key) => (picked = key)} onpickfolded={(keys) => (folded = `${keys.length} games`)} />
    </div>
    <span class="picked">Folded part picked: {folded}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Segments the bar cannot draw (unknown or zero size): listed, and picked from their names in the legend" stack>
    <DiskMeter label="Library drive" segments={UNDRAWN} total={TOTAL} onpick={(key) => (undrawn = key)} />
    <span class="picked">Picked: {undrawn}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Unknown total: a broken outline, nothing filled; and sizes not measured (a lower bound, an unknown)" stack>
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

  <ShowcaseRow caption="Settings rows: each base control named by the label and described by the hint" stack>
    <div class="rows">
      <SettingsRow label="Launch on startup" hint="The launcher opens when you sign in to the system">
        {#snippet children({ labelId, hintId })}
          <Toggle checked={startup} labelledby={labelId} describedby={hintId} onchange={(on) => (startup = on)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Language" hint="The language of the interface">
        {#snippet children({ labelId, hintId })}
          <Select options={LANGUAGES} bind:value={language} labelledby={labelId} describedby={hintId} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Download threads" hint="How many parts of a file come at once" labelFor="{uid}-threads">
        {#snippet children({ hintId })}
          <IntegerInput id="{uid}-threads" value={threads} min={1} max={16} describedby={hintId} onchange={(value) => (threads = value)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Animations" hint="Smooth transitions between screens" note="The system asks for reduced motion: animations stay off">
        {#snippet children({ labelId, hintId })}
          <Toggle checked={motion} labelledby={labelId} describedby={hintId} onchange={(on) => (motion = on)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="In-game overlay" hint="A panel over the running game" disabledReason="Not available in the portable build" testid="kit-row-disabled">
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle checked={false} labelledby={labelId} describedby={hintId} {disabled} onchange={() => {}} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Record gameplay" hint="Saves a video of the game" disabledReason="The recorder is not installed" testid="kit-row-disabled-link">
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle checked labelledby={labelId} describedby={hintId} {disabled} onchange={() => {}} />
        {/snippet}
        {#snippet below()}
          <TextLink tone="muted" onclick={() => (why = !why)}>How to install the recorder</TextLink>
          {#if why}<p class="picked">Install it from the downloads page, then restart the launcher.</p>{/if}
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Interface scale" hint="The size of everything in the window" stack>
        {#snippet children({ labelId, hintId })}
          <Slider position={scale} max={SCALES.length - 1} labelledby={labelId} fill valueText={SCALES[scale]} startText={SCALES[0]} endText={SCALES.at(-1)} describedby={hintId} onchange={(at) => (scale = at)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Library folders" hint="Where games are installed; the first is the default">
        {#snippet below({ labelId })}
          <ul class="folders" aria-labelledby={labelId}>
            <li>
              <span>D:\Games</span>
              <DiskMeter label="D:\Games" segments={DISK.slice(0, 1)} total={TOTAL} free={273 * GB} compact />
            </li>
          </ul>
        {/snippet}
      </SettingsRow>
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Stacked rows: each control under the text at its own width from the start; the slider asks to fill the text's measure" stack>
    <div class="rows">
      <SettingsRow label="Launch on startup" hint="A switch keeps its own width" stack testid="kit-stack-toggle">
        {#snippet children({ labelId, hintId })}
          <Toggle checked={stackOn} labelledby={labelId} describedby={hintId} onchange={(on) => (stackOn = on)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Language" hint="A select keeps its 22rem" stack testid="kit-stack-select">
        {#snippet children({ labelId, hintId })}
          <Select options={LANGUAGES} bind:value={stackLanguage} labelledby={labelId} describedby={hintId} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Cache" hint="A button keeps its own width" stack testid="kit-stack-button">
        {#snippet children()}
          <Button onclick={() => {}}>Clear the cache</Button>
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Interface scale" hint="A slider has no width of its own: it fills the measure" stack testid="kit-stack-slider">
        {#snippet children({ labelId, hintId })}
          <Slider position={stackScale} max={SCALES.length - 1} labelledby={labelId} fill valueText={SCALES[stackScale]} describedby={hintId} onchange={(at) => (stackScale = at)} />
        {/snippet}
      </SettingsRow>
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Rows of the downloads tab: each control named by the label, described by the hint, and off with the row's reason" stack>
    <div class="rows">
      <SettingsRow label="Download speed limit" hint="Maximum incoming traffic speed" testid="kit-row-limit">
        {#snippet children({ labelId, hintId })}
          <RateLimitInput value={limit} presets={[10, 25, 50]} labelledby={labelId} describedby={hintId} onchange={(bytes) => (limit = bytes)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Connection" hint="How the launcher reaches the network" testid="kit-row-mode">
        {#snippet children({ labelId, hintId })}
          <SegmentedControl options={ROUTES} labelledby={labelId} describedby={hintId} bind:value={route} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Schedule" hint="Outside the windows, downloads and seeding stand still" testid="kit-row-schedule">
        {#snippet below({ labelId, hintId })}
          <ScheduleEditor {windows} labelledby={labelId} describedby={hintId} onchange={(next) => (windows = next)} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Schedule (locked)" hint="The same editor in a row with a reason" disabledReason="Managed by your organisation" testid="kit-row-schedule-disabled">
        {#snippet below({ labelId, hintId, disabled })}
          <ScheduleEditor {windows} labelledby={labelId} describedby={hintId} {disabled} onchange={() => {}} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Upload speed limit" hint="Maximum outgoing traffic speed" disabledReason="Set by the play mode while a game runs" testid="kit-row-limit-disabled">
        {#snippet children({ labelId, hintId, disabled })}
          <RateLimitInput value={3 * 1024 * 1024} presets={[1, 5, 10]} labelledby={labelId} describedby={hintId} {disabled} onchange={() => {}} />
        {/snippet}
      </SettingsRow>
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Bar chart: a week of hours on a round hour scale, a value over each bar, today set apart" stack>
    <div class="chart">
      <BarChart label="Play time by day" buckets={WEEK} format={time} scale={secondsScale(Math.max(...WEEK.map((day) => day.value)))} columns={['Day', 'Play time']} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="With a selection: a radio group, arrows and Home / End, the chosen bar labelled" stack>
    <div class="chart">
      <BarChart label="Weeks" buckets={WEEKS} format={time} selectable bind:selected={week} values="selected" height="6.4rem" />
    </div>
    <span class="picked">Chosen: {WEEKS.find((item) => item.key === week)?.label}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="Selectable before anything is chosen: a radio group, the Tab stop on the current week, no table" stack>
    <div class="chart">
      <BarChart label="Weeks" buckets={WEEKS} format={time} selectable bind:selected={notYet} values="selected" height="6.4rem" />
    </div>
    <span class="picked">Chosen: {WEEKS.find((item) => item.key === notYet)?.label ?? 'nothing yet'}</span>
  </ShowcaseRow>

  <ShowcaseRow caption="A value over the scale: drawn to the top, its label and the table say the real value" stack>
    <div class="chart">
      <BarChart label="Play time by day" buckets={WEEK} format={time} scale={{ max: 4 * HOUR, step: HOUR }} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="One huge bar: the small ones stay hairlines" stack>
    <div class="chart">
      <BarChart label="Play time by day" buckets={HUGE} format={time} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Nothing above zero: the caller's empty state" stack>
    <div class="chart">
      <BarChart label="Play time by day" buckets={NOTHING} format={time}>
        {#snippet empty()}
          <EmptyState size="inline" title="No play this week" />
        {/snippet}
      </BarChart>
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

  .rows,
  .chart {
    max-width: 72rem;
  }

  .folders {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .folders li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 24rem);
    align-items: center;
    gap: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
