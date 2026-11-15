<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import { t } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import type { Download } from '../../lib/services/downloads';
  import { completed, requestDownloadAction } from '../../lib/stores/downloads';
  import { installationsByDownload } from '../../lib/stores/install';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import CompletedRow from './CompletedRow.svelte';
  import { CLEAR_ASKS_ABOVE, canClear, completedKind, completedPart, type CompletedPart } from './completedRow';

  /**
   * One of the two sections of the completed downloads, newest first: "Installing" (their installation
   * runs or waits for the player) or "Completed" (the rest, with "Clear the list"). The page mounts
   * each where it wants it; a section with no download renders nothing.
   *
   * Focus: when a row leaves under the player's press (Remove, a Retry that moves it to "Installing",
   * Clear), focus goes to the same download where it went, else the row now in its place, else the one
   * above, else this section's heading, else `away` (the page's heading). It waits for the row to leave:
   * Go may answer before its event does.
   */
  let {
    part,
    onopen,
    oninstall,
    onannounce,
    away,
  }: {
    part: CompletedPart;
    onopen: (id: string) => void;
    oninstall: (id: string) => void;
    /** Says what the player did, in the page's one polite live region. */
    onannounce?: (text: string) => void;
    /** Where focus goes when nothing of this section is left to take it. */
    away?: () => HTMLElement | null | undefined;
  } = $props();

  const uid = $props.id();
  const headingId = $derived(part === 'installing' ? 'downloads-installing-heading' : 'downloads-completed-heading');

  const rows = $derived(
    $completed
      .map((download) => {
        const installation = $installationsByDownload.get(download.id);
        return { download, installation, kind: completedKind(download, installation) };
      })
      .filter((row) => completedPart(row.kind) === part),
  );
  const clearable = $derived(rows.filter((row) => canClear(row.download, row.kind)).map((row) => row.download.id));

  let list = $state<HTMLElement>();

  const ROW = 'data-completed-row';
  const rowElements = (root: ParentNode | undefined) => [...(root?.querySelectorAll<HTMLElement>(`[${ROW}]`) ?? [])];
  const control = (row: HTMLElement, prefer: 'lead' | 'more') =>
    row.querySelector<HTMLElement>(`[data-row-control="${prefer}"]`) ?? row.querySelector<HTMLElement>('[data-row-control="more"]');

  function target(id: string, order: readonly string[]): HTMLElement | null | undefined {
    const moved = rowElements(document).find((row) => row.getAttribute(ROW) === id);
    if (moved) return control(moved, 'lead');
    const shown = new Map(rowElements(list).map((row) => [row.getAttribute(ROW) ?? '', row]));
    const at = order.indexOf(id);
    const candidates = at < 0 ? [] : [...order.slice(at + 1), ...order.slice(0, at).reverse()];
    for (const other of candidates) {
      const row = shown.get(other);
      if (row) return control(row, 'more');
    }
    const heading = document.getElementById(headingId);
    if (heading?.isConnected) return heading;
    return away?.();
  }

  interface Leaving {
    order: string[];
    hand: ReturnType<typeof focusHandOff>;
    armed: boolean;
  }

  // Plain maps: what the rows do decides when they settle, not a render of their own.
  const leaving = new Map<string, Leaving>();
  let cleared: { hand: ReturnType<typeof focusHandOff>; ids: Set<string> } | null = null;

  function onleave(id: string) {
    const entry: Leaving = { order: rows.map((row) => row.download.id), hand: focusHandOff(), armed: false };
    leaving.set(id, entry);
    return (ok: boolean) => {
      if (leaving.get(id) !== entry) return;
      if (!ok) {
        leaving.delete(id);
        return;
      }
      entry.armed = true;
      settle(new Set(rows.map((row) => row.download.id)));
    };
  }

  function settle(present: ReadonlySet<string>) {
    for (const [id, entry] of leaving) {
      if (!entry.armed || present.has(id)) continue;
      leaving.delete(id);
      void entry.hand(() => target(id, entry.order));
    }
    if (cleared && [...cleared.ids].every((id) => !present.has(id))) {
      const { hand } = cleared;
      cleared = null;
      // The Clear button is still there with rows left; with none, the section went from under it.
      if (present.size === 0) void hand(() => away?.());
    }
  }

  $effect(() => {
    settle(new Set(rows.map((row) => row.download.id)));
  });

  async function removeRow(download: Download) {
    const answer = onleave(download.id);
    const outcome = await requestDownloadAction(download.id, 'remove');
    answer(outcome.ok);
    if (outcome.ok) onannounce?.($t('downloads.done.removed', { name: download.name }));
  }

  let clearing = $state(false);
  let confirmOpen = $state(false);

  function askClear() {
    if (clearing || clearable.length === 0) return;
    if (clearable.length > CLEAR_ASKS_ABOVE) confirmOpen = true;
    else void clearNow();
  }

  // One request per download, one after another; the outcome is said once, with what Go refused.
  async function clearNow() {
    if (clearing) return;
    const ids = [...clearable];
    if (ids.length === 0) return;
    clearing = true;
    const hand = focusHandOff();
    let removed = 0;
    let refused = 0;
    const gone = new Set<string>();
    for (const id of ids) {
      const outcome = await requestDownloadAction(id, 'remove', { toast: false });
      if (outcome.ok) {
        removed++;
        gone.add(id);
      } else refused++;
    }
    clearing = false;
    const said = [
      removed > 0 ? $t('downloads.done.cleared', { count: removed }) : '',
      refused > 0 ? $t('downloads.done.clearFailed', { count: refused }) : '',
    ].filter(Boolean);
    onannounce?.(said.join('. '));
    if (gone.size > 0) {
      cleared = { hand, ids: gone };
      settle(new Set(rows.map((row) => row.download.id)));
    }
  }

  const clearPrompt = $derived({
    title: $t('downloads.done.clearTitle'),
    text: $t('downloads.done.clearText', { count: clearable.length }),
    confirm: $t('downloads.done.clearConfirm'),
    busy: $t('downloads.done.clearBusy'),
    cancel: $t('downloads.done.clearKeep'),
  });

  const clearButton = () => document.querySelector<HTMLElement>(`[data-clear="${uid}"]`);
</script>

{#snippet clear()}
  <Button
    size="sm"
    variant="ghost"
    disabled={clearable.length === 0}
    reason={clearable.length === 0 ? $t('downloads.done.clearNothing') : ''}
    busy={clearing}
    data-clear={uid}
    onclick={askClear}
  >
    {$t('downloads.done.clear')}
  </Button>
{/snippet}

{#if rows.length > 0}
  <section class="section" aria-labelledby={headingId} data-testid={part === 'installing' ? 'downloads-installing' : 'downloads-completed'}>
    <SectionHeader
      id={headingId}
      title={part === 'installing' ? $t('downloads.done.installingTitle') : $t('downloads.done.title')}
      count={rows.length}
      actions={part === 'completed' ? clear : undefined}
    />
    <ul class="rows" bind:this={list}>
      {#each rows as row (row.download.id)}
        <CompletedRow
          download={row.download}
          installation={row.installation}
          kind={row.kind}
          {onopen}
          {oninstall}
          onremove={removeRow}
          {onleave}
        />
      {/each}
    </ul>
  </section>
{/if}

{#if confirmOpen}
  <ConfirmModal
    prompt={clearPrompt}
    onconfirm={clearNow}
    onclose={() => (confirmOpen = false)}
    errorText={(err) => installErrorText(err)}
    returnFocus={() => clearButton() ?? away?.()}
  />
{/if}

<style>
  .section {
    margin-bottom: var(--space-8);
    max-width: 140rem;
  }

  .rows {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
