<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import { t } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import type { Download } from '../../lib/services/downloads';
  import type { Installation } from '../../lib/services/install';
  import { completed, requestDownloadAction } from '../../lib/stores/downloads';
  import { installationsByDownload } from '../../lib/stores/install';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import type { LeaveTicket } from '../../lib/components/downloadPanel';
  import CompletedRow from './CompletedRow.svelte';
  import { CLEAR_ASKS_ABOVE, canClear, completedKind, completedPart, removeAsks, type CompletedKind, type CompletedPart } from './completedRow';
  import { handOffInView, pageHeading, pressed, returnInView } from './rowFocus';
  import { downloadsSectionIds } from './sections';

  /**
   * One of the two sections of the completed downloads, newest first: "Installing" (their installation
   * runs or waits for the player) or "Completed" (the rest, with "Clear the list"). The page mounts
   * each where it wants it; a section with no download renders nothing.
   *
   * Focus: when a row leaves under the player's press (Remove, a Retry that moves it to "Installing"),
   * it goes by the screen's rule (rowFocus.ts) to the same download where it went, else the row now in
   * its place, else the one above, else this section's heading, else the page's. It waits for the row to
   * leave: Go may answer before its event does.
   */
  let {
    part,
    onopen,
    oninstall,
    onannounce,
  }: {
    part: CompletedPart;
    onopen: (id: string) => void;
    oninstall: (id: string) => void;
    /** Says what the player did, in the page's one polite live region. */
    onannounce?: (text: string) => void;
  } = $props();

  const uid = $props.id();
  const headingId = $derived(downloadsSectionIds[part]);

  interface Row {
    download: Download;
    installation: Installation | undefined;
    kind: CompletedKind;
  }

  // A row keeps its object while its download and its installation are the same objects: a progress
  // tick of another download changes `$completed` and must not make every row redo its work.
  let lastRows = new Map<string, Row>();

  const rows = $derived.by(() => {
    const next = new Map<string, Row>();
    for (const download of $completed) {
      const installation = $installationsByDownload.get(download.id);
      const kept = lastRows.get(download.id);
      const row =
        kept && kept.download === download && kept.installation === installation
          ? kept
          : { download, installation, kind: completedKind(download, installation) };
      next.set(download.id, row);
    }
    lastRows = next;
    return Array.from(next.values()).filter((row) => completedPart(row.kind) === part);
  });
  const clearable = $derived(rows.filter((row) => canClear(row.download, row.kind)).map((row) => row.download.id));

  interface Leaving {
    ticket: LeaveTicket;
    armed: boolean;
  }

  // Plain maps: what the rows do decides when they settle, not a render of their own.
  const leaving = new Map<string, Leaving>();
  let cleared: { hand: ReturnType<typeof focusHandOff>; ids: Set<string> } | null = null;

  function onleave(id: string) {
    const entry: Leaving = { ticket: pressed(id, { follow: 'first', prefer: '[data-row-control="more"]' }), armed: false };
    leaving.set(id, entry);
    return (ok: boolean) => {
      if (leaving.get(id) !== entry) return;
      if (!ok) {
        leaving.delete(id);
        entry.ticket.drop();
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
      void entry.ticket.handOn();
    }
    if (cleared && [...cleared.ids].every((id) => !present.has(id))) {
      const { hand } = cleared;
      cleared = null;
      // The Clear button is still there with rows left; with none, the section went from under it.
      if (present.size === 0) void handOffInView(hand, pageHeading);
    }
  }

  $effect(() => {
    settle(new Set(rows.map((row) => row.download.id)));
  });

  /**
   * The download a "Remove from list" asks about, by id: the dialog says what goes and closes when the
   * download leaves this section (Go removed it, or its install began).
   */
  let asking = $state<{ id: string; answer: (ok: boolean) => void; ticket: LeaveTicket; done: boolean } | null>(null);
  const askingDownload = $derived(asking ? rows.find((row) => row.download.id === asking?.id)?.download : undefined);

  $effect(() => {
    if (asking && !askingDownload) untrack(closeRemove);
  });

  function removeRow(download: Download) {
    const kind = rows.find((row) => row.download.id === download.id)?.kind;
    const answer = onleave(download.id);
    const ticket = leaving.get(download.id)?.ticket;
    if (kind && ticket && removeAsks(kind)) asking = { id: download.id, answer, ticket, done: false };
    else void removeNow(download, answer);
  }

  async function removeNow(download: Download, answer: (ok: boolean) => void) {
    const outcome = await requestDownloadAction(download.id, 'remove');
    answer(outcome.ok);
    if (outcome.ok) onannounce?.($t('downloads.done.removed', { name: download.name }));
  }

  // The dialog shows a refusal itself, by its code, and stays open for another try.
  async function confirmRemove() {
    const current = asking;
    const download = askingDownload;
    if (!current || !download) return;
    const outcome = await requestDownloadAction(current.id, 'remove', { toast: false });
    if (!outcome.ok) throw outcome.error;
    current.done = true;
    current.answer(true);
    onannounce?.($t('downloads.done.removed', { name: download.name }));
  }

  function closeRemove() {
    if (asking && !asking.done) asking.answer(false);
    asking = null;
  }

  const removePrompt = $derived(
    askingDownload
      ? {
          title: $t('downloads.done.removeTitle', { name: askingDownload.name }),
          text: $t('downloads.done.removeText'),
          note: askingDownload.seeding ? $t('downloads.done.removeSeedingNote') : undefined,
          confirm: $t('downloads.done.removeConfirm'),
          busy: $t('downloads.done.removeBusy'),
          cancel: $t('downloads.done.removeKeep'),
        }
      : null,
  );

  let clearing = $state(false);
  let confirmOpen = $state(false);

  function askClear() {
    if (clearing || clearable.length === 0) return;
    if (clearable.length > CLEAR_ASKS_ABOVE) confirmOpen = true;
    else void clearNow();
  }

  // One request per download, one after another; the outcome is said once, with what Go refused: in the
  // live region, and a refusal on screen too, as rows that stay say nothing to the eye of why.
  async function clearNow() {
    if (clearing) return;
    const ids = [...clearable];
    if (ids.length === 0) return;
    clearing = true;
    const hand = focusHandOff();
    let removed = 0;
    let refused = 0;
    let reason: unknown = null;
    const gone = new Set<string>();
    for (const id of ids) {
      const outcome = await requestDownloadAction(id, 'remove', { toast: false });
      if (outcome.ok) {
        removed++;
        gone.add(id);
      } else {
        refused++;
        reason ??= outcome.error;
      }
    }
    clearing = false;
    const said = [
      removed > 0 ? $t('downloads.done.cleared', { count: removed }) : '',
      refused > 0 ? $t('downloads.done.clearFailed', { count: refused }) : '',
    ].filter(Boolean);
    onannounce?.(said.join('. '));
    if (refused > 0) toast($t('downloads.done.clearFailedReason', { count: refused, reason: installErrorText(reason) }), 'danger');
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
    <ul class="rows">
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

{#if asking && removePrompt}
  {@const ticket = asking.ticket}
  <ConfirmModal
    prompt={removePrompt}
    onconfirm={confirmRemove}
    errorText={installErrorText}
    onclose={closeRemove}
    returnFocus={() => ticket.target()}
  />
{/if}

{#if confirmOpen}
  <ConfirmModal
    prompt={clearPrompt}
    onconfirm={clearNow}
    onclose={() => (confirmOpen = false)}
    errorText={(err) => installErrorText(err)}
    returnFocus={() => returnInView(clearButton() ?? pageHeading())}
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
