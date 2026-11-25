<script lang="ts">
  import { Eye, LayoutGrid, List, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Checkbox from '../../lib/components/Checkbox.svelte';
  import HexColorField from '../../lib/components/HexColorField.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import OptionList from '../../lib/components/OptionList.svelte';
  import type { OptionItem } from '../../lib/components/optionList';
  import PathField from '../../lib/components/PathField.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Slider from '../../lib/components/Slider.svelte';
  import Select from '../../lib/components/Select.svelte';
  import type { SelectOption } from '../../lib/components/select';
  import type { SegmentOption } from '../../lib/components/segmented';
  import Tabs from '../../lib/components/Tabs.svelte';
  import TextArea from '../../lib/components/TextArea.svelte';
  import TextField from '../../lib/components/TextField.svelte';
  import { tabPanel, type TabItem } from '../../lib/components/tabs';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { validAccent } from '../../lib/theme/accent';
  import { rateLimitLabel } from '../../lib/utils/format';
  import { SPEED_LIMIT_NO_LIMIT, speedLimitAt, speedLimitPosition } from '../../lib/utils/speedLimit';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const uid = $props.id();

  let cloud = $state(true);
  let news = $state(false);
  // Held by the page and asked for through onchange, as a caller that saves the choice does.
  let parts = $state([true, false, true]);
  const all = $derived(parts.every(Boolean) ? true : parts.some(Boolean) ? 'mixed' : false);

  // The speed limit as a caller holds it: the stored bytes/s, a draft while the thumb is dragged, the
  // step being saved, and a save that takes a second (busy) and refuses 1000 MB/s, so the thumb goes
  // back to what is stored.
  const MB = 1024 ** 2;
  let limit = $state(12 * MB);
  let draft = $state<number | null>(null);
  let saving = $state<number | null>(null);
  const limitShown = $derived(draft ?? saving ?? speedLimitPosition(limit));
  const limitText = $derived(rateLimitLabel(draft !== null || saving !== null ? speedLimitAt(limitShown) : limit));
  function saveLimit(position: number) {
    saving = position;
    setTimeout(() => {
      if (speedLimitAt(position) !== 1000 * MB) limit = speedLimitAt(position);
      saving = null;
    }, 1000);
  }

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

  let email = $state('');
  let handle = $state('taken_name');
  // An error that comes after an action is announced; the one above was there when the page opened.
  let checked = $state('');
  let host = $state('');
  let secret = $state('fake-pass');
  let revealed = $state(false);
  let search = $state('a query to clear');

  // A picker that takes a second, as the system's folder dialog does, and answers the next folder.
  const FOLDERS = ['D:/Games', 'E:/A drive with a long folder name/Games bought on sale/Typhon Library/Installed games'];
  let folder = $state('');
  let destination = $state('C:/Games/Hades');
  let destinationError = $state('');
  function pickFolder(): Promise<void> {
    return new Promise((resolve) => {
      setTimeout(() => {
        folder = FOLDERS[(FOLDERS.indexOf(folder) + 1) % FOLDERS.length];
        resolve();
      }, 1000);
    });
  }

  const SAVE_FOLDERS: OptionItem[] = [
    { id: 'documents', title: 'C:/Users/me/Documents/My Games/Hades', note: 'Changed today', fact: '12 MB' },
    { id: 'appdata', title: 'C:/Users/me/AppData/Local/Supergiant Games/A folder whose name runs on for long enough to wrap/Hades/Profiles', fact: '3 MB' },
    { id: 'old', title: 'D:/Old games/Hades', disabledReason: 'The drive is not connected.' },
    { id: 'cloud', title: 'E:/Cloud/Hades' },
  ];
  let saveFolder = $state('');
  const MODES: OptionItem[] = [
    { id: 'keep', title: 'Keep the files', note: 'The game leaves the library; its folder stays.' },
    { id: 'delete', title: 'Delete the files', note: 'Saves inside the folder go with it.' },
  ];
  let mode = $state('keep');

  function browseDestination() {
    destination = 'D:/Games/Hades';
  }

  let note = $state('');
  let message = $state('The game closes right after the logo.\nIt worked yesterday.');
  let messageError = $state('');
  let sentNotes = $state(0);
  const noteCounter = {
    max: 60,
    text: (count: number, max: number) => `${count} of ${max}`,
    limitText: (count: number, max: number) => (count > max ? `${count - max} characters over the limit` : `${max - count} characters left`),
  };
  function sendNote(event: KeyboardEvent) {
    if (event.key !== 'Enter' || !(event.ctrlKey || event.metaKey)) return;
    event.preventDefault();
    sentNotes += 1;
  }
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

  <ShowcaseRow caption="Text fields: a hint, required, an error shown on opening, an error after Check, read-only, disabled, disabled with a reason, named by a label beside it" stack>
    <span class="text"><TextField label="E-mail" type="email" autocomplete="email" required hint="We send the confirmation here." bind:value={email} /></span>
    <span class="text"><TextField label="Username" autocomplete="username" maxlength={24} error="This username is taken." bind:value={handle} /></span>
    <span class="text"><TextField label="Display name" bind:value={host} error={checked} /></span>
    <span><Button size="sm" onclick={() => (checked = host.trim() ? '' : 'Enter a display name.')}>Check</Button></span>
    <span class="text"><TextField label="Signed in as" value="player_one" readonly /></span>
    <span class="text"><TextField label="Disabled" value="proxy.example.test" disabled /></span>
    <span class="text"><TextField label="Proxy host" value="proxy.example.test" disabledReason="The proxy is off." hint="A host name or an address." /></span>
    <span class="field-label" id="{uid}-beside">Named by the label beside it</span>
    <span class="text"><TextField labelledby="{uid}-beside" placeholder="Type here" /></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Text fields with a control at the end: reveal a password, clear" stack>
    <span class="text">
      <TextField label="Password" type={revealed ? 'text' : 'password'} autocomplete="current-password" bind:value={secret}>
        {#snippet trailing()}
          <IconButton size="sm" label="Show password" pressed={revealed} onclick={() => (revealed = !revealed)}><Eye size="1.6rem" /></IconButton>
        {/snippet}
      </TextField>
    </span>
    <span class="text">
      <TextField label="Filter" bind:value={search}>
        {#snippet trailing()}
          {#if search}<IconButton size="sm" label="Clear" onclick={() => (search = '')}><X size="1.6rem" /></IconButton>{/if}
        {/snippet}
      </TextField>
    </span>
  </ShowcaseRow>

  <ShowcaseRow caption="Multi-line fields: a counter said near the limit and sent with Ctrl/Cmd+Enter, growing with its text, an error after Check, read-only, disabled with a reason" stack>
    <span class="text"><TextArea label="Note" rows={3} counter={noteCounter} hint={`Sent with Ctrl/Cmd+Enter: ${sentNotes}`} bind:value={note} onkeydown={sendNote} /></span>
    <span class="text"><TextArea label="Details" rows={3} grow={8} placeholder="What you did and what happened" error={messageError} bind:value={message} /></span>
    <span><Button size="sm" onclick={() => (messageError = message.trim() ? '' : 'Describe the problem.')}>Check</Button></span>
    <span class="text"><TextArea label="Sent" rows={2} value="Already sent, read-only while the answer comes." readonly /></span>
    <span class="text"><TextArea label="Reply" rows={2} disabledReason="The request is closed." /></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Path fields: none chosen, then a long path after Browse (busy for a second) with a fact under it; typed as well as browsed, with an error after Check; disabled with a reason" stack>
    <span class="text">
      <PathField label="Library folder" placeholder="No folder chosen" browseLabel="Browse" onbrowse={pickFolder} value={folder}>
        {#snippet facts()}{#if folder}<span class="field-label">Drive {folder.slice(0, 1)} · 412 GB free of 1 TB</span>{/if}{/snippet}
      </PathField>
    </span>
    <span class="text"><PathField label="Install to" editable hint="A folder on any drive." error={destinationError} browseLabel="Browse" onbrowse={browseDestination} bind:value={destination} /></span>
    <span><Button size="sm" onclick={() => (destinationError = destination.trim() ? '' : 'Choose where to install.')}>Check</Button></span>
    <span class="text"><PathField label="Move to" value="D:/Games/Hades" disabledReason="The game is running." browseLabel="Browse" onbrowse={() => {}} /></span>
  </ShowcaseRow>

  <ShowcaseRow caption="Option lists: none chosen yet, a second line, a fact at the end, a long path that wraps, a row refused with its reason; the page holds the choice through onchange; rows drawn by the caller" stack>
    <span class="field-label" id="{uid}-saves">Save folder (chosen: {saveFolder || 'none'})</span>
    <span class="text"><OptionList labelledby="{uid}-saves" options={SAVE_FOLDERS} value={saveFolder} onchange={(id) => (saveFolder = id)} /></span>
    <span class="text">
      <OptionList label="When removing" options={MODES} bind:value={mode}>
        {#snippet row(option)}<span class="mode-title">{option.title}</span> <span class="field-label">{option.note}</span>{/snippet}
      </OptionList>
    </span>
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

  <ShowcaseRow caption="Sliders: the speed limit from a stored 12 MB/s, off the steps (1000 MB/s is refused); disabled" stack>
    <span class="slider">
      <Slider
        position={limitShown}
        max={SPEED_LIMIT_NO_LIMIT}
        label="Speed limit"
        valueText={limitText}
        startText={rateLimitLabel(speedLimitAt(0))}
        endText={rateLimitLabel(0)}
        busy={saving !== null}
        oninput={(next) => (draft = next)}
        onchange={saveLimit}
      />
    </span>
    <span class="slider">
      <Slider position={7} max={SPEED_LIMIT_NO_LIMIT} label="Locked limit" valueText={rateLimitLabel(speedLimitAt(7))} disabled onchange={() => {}} />
    </span>
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

  .slider {
    display: block;
    max-width: 32rem;
  }

  .search {
    display: flex;
    width: 28rem;
  }

  .text {
    display: block;
    width: 32rem;
  }

  .mode-title {
    display: block;
    font-size: var(--font-sm);
  }
</style>
