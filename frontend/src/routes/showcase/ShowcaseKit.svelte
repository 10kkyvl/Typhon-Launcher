<script lang="ts">
  import DiskMeter from '../../lib/components/DiskMeter.svelte';
  import { gameTone, type MeterSegment } from '../../lib/components/diskMeter';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Select from '../../lib/components/Select.svelte';
  import type { SelectOption } from '../../lib/components/select';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import Slider from '../../lib/components/Slider.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
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
      <SettingsRow label="In-game overlay" hint="A panel over the running game" disabledReason="Not available in the portable build">
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle checked={false} labelledby={labelId} describedby={hintId} {disabled} onchange={() => {}} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label="Interface scale" hint="The size of everything in the window" stack>
        {#snippet children({ hintId })}
          <Slider position={scale} max={SCALES.length - 1} label="Interface scale" hideLabel valueText={SCALES[scale]} startText={SCALES[0]} endText={SCALES.at(-1)} describedby={hintId} onchange={(at) => (scale = at)} />
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
</ShowcaseSection>

<style>
  .picked {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .compact {
    max-width: 32rem;
  }

  .rows {
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
