<script lang="ts">
  import { LayoutGrid, List } from '@lucide/svelte';
  import Checkbox from '../../lib/components/Checkbox.svelte';
  import HexColorField from '../../lib/components/HexColorField.svelte';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Select from '../../lib/components/Select.svelte';
  import type { SelectOption } from '../../lib/components/select';
  import type { SegmentOption } from '../../lib/components/segmented';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { tabPanel, type TabItem } from '../../lib/components/tabs';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { validAccent } from '../../lib/theme/accent';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const uid = $props.id();

  let cloud = $state(true);
  let news = $state(false);
  // Held by the page and asked for through onchange, as a caller that saves the choice does.
  let parts = $state([true, false, true]);
  const all = $derived(parts.every(Boolean) ? true : parts.some(Boolean) ? 'mixed' : false);

  let overlay = $state(true);
  let autostart = $state(false);

  const LANGUAGES: SelectOption[] = [
    { id: 'en', label: 'English' },
    { id: 'ru', label: 'Russian' },
    { id: 'de', label: 'German', disabled: true },
    { id: 'long', label: 'A language whose name is long enough to show how the list holds it' },
  ];
  let language = $state('en');
  let held = $state('');

  const VIEWS: SegmentOption[] = [
    { id: 'grid', label: 'Grid' },
    { id: 'list', label: 'List' },
  ];
  let view = $state('grid');
  const FILTERS: SegmentOption[] = [
    { id: 'all', label: 'All', count: 120 },
    { id: 'installed', label: 'Installed', count: 12 },
    { id: 'favourites', label: 'Favourites', count: 0 },
    { id: 'hidden', label: 'Hidden', disabled: true },
  ];
  let filter = $state('all');
  let icons = $state('grid');

  const TABS: TabItem[] = [
    { id: 'about', label: 'About' },
    { id: 'reviews', label: 'Reviews', count: 31 },
    { id: 'files', label: 'Files' },
    { id: 'mods', label: 'Mods', disabled: true },
  ];
  let tab = $state('about');
  let pill = $state('reviews');

  let query = $state('');
  let island = $state('');

  let accent = $state('#4C8DFF');
  let frame = $state('#12');
  const frameValid = $derived(validAccent(frame));
  let optional = $state('');

  let fps = $state(60);
</script>

<ShowcaseSection title="Fields" note="Checkboxes, switches, selects, segmented controls, tabs, search, colour and number fields. The page holds every value, as a real caller does.">
  <ShowcaseRow caption="Checkboxes" stack>
    <Checkbox bind:checked={cloud} label="Sync saves to the cloud" description="Checked or not, written through bind:checked." />
    <Checkbox
      checked={all}
      label="All parts of the game"
      aside="48.2 GB"
      onchange={(next) => (parts = parts.map(() => next))}
    />
    {#each parts as part, index (index)}
      <span class="nested">
        <Checkbox checked={part} label={`Part ${index + 1}`} aside={`${(index + 1) * 8} GB`} onchange={(next) => (parts[index] = next)} />
      </span>
    {/each}
    <Checkbox bind:checked={news} label="Disabled" disabled />
    <Checkbox checked label="Disabled with a reason" disabled reason="The game is running" />
  </ShowcaseRow>

  <ShowcaseRow caption="Switches" stack>
    <div class="switch-row">
      <span id="{uid}-overlay">In-game overlay</span>
      <Toggle labelledby="{uid}-overlay" checked={overlay} onchange={(next) => (overlay = next)} />
    </div>
    <div class="switch-row">
      <span class="switch-text">
        <span id="{uid}-autostart">Start with the system</span>
        <span class="hint" id="{uid}-autostart-hint">Bound with bind:checked, described by this line.</span>
      </span>
      <Toggle labelledby="{uid}-autostart" describedby="{uid}-autostart-hint" bind:checked={autostart} />
    </div>
    <div class="switch-row">
      <span id="{uid}-locked">Disabled</span>
      <Toggle labelledby="{uid}-locked" checked disabled onchange={() => {}} />
    </div>
  </ShowcaseRow>

  <ShowcaseRow caption="Selects">
    <label class="field-label" for="{uid}-language">Language</label>
    <Select id="{uid}-language" options={LANGUAGES} bind:value={language} />
    <span class="field-label" id="{uid}-held">Held by the page, with a placeholder</span>
    <Select labelledby="{uid}-held" options={LANGUAGES} value={held} placeholder="Pick a language" onchange={(id) => (held = id)} />
    <span class="field-label" id="{uid}-off">Disabled</span>
    <Select labelledby="{uid}-off" options={LANGUAGES} value="en" disabled onchange={() => {}} />
  </ShowcaseRow>

  <ShowcaseRow caption="Segmented controls">
    <span class="field-label" id="{uid}-view">View</span>
    <SegmentedControl labelledby="{uid}-view" options={VIEWS} bind:value={view} />
    <span class="field-label" id="{uid}-filter">Filter (pill, counts, a disabled option)</span>
    <SegmentedControl labelledby="{uid}-filter" variant="pill" options={FILTERS} bind:value={filter} />
    <span class="field-label" id="{uid}-icons">Icons</span>
    <SegmentedControl labelledby="{uid}-icons" options={VIEWS} bind:value={icons}>
      {#snippet item(option)}
        {#if option.id === 'grid'}<LayoutGrid size="1.6rem" />{:else}<List size="1.6rem" />{/if}
      {/snippet}
    </SegmentedControl>
    <span class="field-label" id="{uid}-locked-view">Disabled</span>
    <SegmentedControl labelledby="{uid}-locked-view" options={VIEWS} value="list" disabled />
  </ShowcaseRow>

  <ShowcaseRow caption="Tabs" stack>
    <Tabs tabs={TABS} bind:value={tab} idPrefix="{uid}-tabs" label="Game page" />
    <div class="panel" {...tabPanel(`${uid}-tabs`, tab)}>The {tab} tab's panel. Tab moves here from the chosen tab.</div>
    <Tabs tabs={TABS} bind:value={pill} variant="pill" idPrefix="{uid}-pills" label="Game page, pill" />
    <div class="panel" {...tabPanel(`${uid}-pills`, pill)}>The {pill} pill's panel.</div>
  </ShowcaseRow>

  <ShowcaseRow caption="Search fields">
    <span class="search"><SearchInput label="Search the showcase" bind:value={query} placeholder="Type a name" /></span>
    <span class="search"><SearchInput label="Searching" value="hollow" loading /></span>
    <span class="search"><SearchInput label="Island search" variant="island" bind:value={island} shortcut="Ctrl K" /></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Colour fields: valid, invalid, optional and empty, disabled" stack>
    <HexColorField label="Accent color" value={accent} onchange={(next) => (accent = next)} />
    <HexColorField label="Frame color" value={frame} describedby={frameValid ? undefined : `${uid}-frame-error`} onchange={(next) => (frame = next)} />
    {#if !frameValid}
      <Notice tone="danger" compact announce={false} id="{uid}-frame-error">Enter a colour as #RRGGBB.</Notice>
    {/if}
    <HexColorField label="Name color" value={optional} optional onchange={(next) => (optional = next)} />
    <HexColorField label="Locked color" value="#22AA66" disabled />
  </ShowcaseRow>

  <ShowcaseRow caption="Number field (10 to 240; a value out of range is refused on Enter or blur)">
    <IntegerInput label="Frame limit" value={fps} min={10} max={240} onchange={(next) => (fps = next)} />
    <span class="field-label">Held value: {fps}</span>
  </ShowcaseRow>
</ShowcaseSection>

<style>
  .nested {
    padding-left: var(--space-6);
  }

  .switch-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    max-width: var(--prose-max);
    font-size: var(--font-md);
    color: var(--text);
  }

  .switch-text {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .hint,
  .field-label {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .panel {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .search {
    display: flex;
    width: 28rem;
  }
</style>
