<script lang="ts">
  import {
    ArrowRightLeft,
    CircleAlert,
    Download,
    History as HistoryIcon,
    PackageCheck,
    PackageX,
    RefreshCw,
    Trash2,
    Undo2,
    Wifi,
  } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { historyArt, type HistoryGameLookup } from '../../lib/history/historyGame';
  import { historyLabel, historyTitleParts, historyWhen } from '../../lib/history/historyText';
  import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
  import { gameArt } from '../../lib/stores/metadata';

  /**
   * One day of the history: its heading (a section heading, focusable by script) and its records,
   * newest first, as a list on a plate of its own. A row says what happened to which game, why when it
   * failed, and when; the game's cover leads it and the kind's icon marks what happened. The game's
   * name, and for the pointer its cover, lead to the game's page while the library holds the game.
   */
  let {
    id,
    label,
    records,
    gameOf,
  }: {
    /** The id of the day's heading, which names the section. */
    id: string;
    label: string;
    records: HistoryRecord[];
    /** The library game of a record (lib/history/historyGame.ts). */
    gameOf: HistoryGameLookup;
  } = $props();

  const icons: Record<Kind, typeof HistoryIcon> = {
    [Kind.$zero]: HistoryIcon,
    [Kind.KindInstalled]: PackageCheck,
    [Kind.KindInstallFailed]: PackageX,
    [Kind.KindUpdated]: RefreshCw,
    [Kind.KindUpdateFailed]: CircleAlert,
    [Kind.KindRolledBack]: Undo2,
    [Kind.KindDownloaded]: Download,
    [Kind.KindUninstalled]: Trash2,
    [Kind.KindRemoved]: Trash2,
    [Kind.KindMoved]: ArrowRightLeft,
    [Kind.KindLanReceived]: Wifi,
  };

  const failed = (kind: Kind) => kind === Kind.KindInstallFailed || kind === Kind.KindUpdateFailed;
</script>

<section class="day" aria-labelledby={id}>
  <SectionHeader {id} title={label} />
  <ul class="rows">
    {#each records as record (record.id)}
      {@const text = historyLabel(record)}
      {@const Icon = icons[record.kind] ?? HistoryIcon}
      {@const when = historyWhen(record.at)}
      {@const game = gameOf(record.gameId)}
      {@const title = historyTitleParts(record)}
      {@const platform = game?.emulation?.platform}
      {@const picture = historyArt(record, game, $gameArt)}
      <li class="row" class:failed={failed(record.kind)} data-history-row={record.id} data-kind={record.kind}>
        <div class="cover">
          <GameLink id={game?.id} name={title.name} repeat>
            <Artwork src={picture.cover} alt="" label={title.name} seed={picture.seed || undefined} ratio="3 / 4" radius="var(--radius-sm)" />
          </GameLink>
        </div>
        <span class="mark" aria-hidden="true"><Icon size="1.6rem" strokeWidth={1.8} /></span>
        <div class="main">
          <div class="head">
            <p class="title">{title.before}<GameLink id={game?.id} name={title.name} />{title.after}</p>
            {#if platform}<ConsoleTag {platform} />{/if}
          </div>
          {#if text.detail}<p class="detail">{text.detail}</p>{/if}
        </div>
        {#if when}
          <!-- The exact time goes to assistive tech as text and to the pointer as a hint; the time element
               takes focus only from the pointer, so the row adds no Tab stop for it. -->
          <Tooltip text={when.exact} describe={false}>
            <time class="when" datetime={record.at} tabindex="-1"><span aria-hidden="true">{when.words}</span><span class="sr-only">{when.exact}</span></time>
          </Tooltip>
        {/if}
      </li>
    {/each}
  </ul>
</section>

<style>
  .rows {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    overflow: hidden;
  }

  .row {
    display: grid;
    grid-template-columns: var(--row-thumb-w) auto minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-3);
    min-height: var(--space-12);
    padding: var(--space-2) var(--space-4) var(--space-2) var(--space-3);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .cover {
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .mark {
    display: grid;
    place-items: center;
    width: var(--space-8);
    height: var(--space-8);
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .row.failed .mark {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    min-width: 0;
  }

  /* Shown whole, wrapping in the row: an ellipsis would hide words that no keyboard could reach. */
  .title {
    margin: 0;
    min-width: 0;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
    overflow-wrap: anywhere;
  }

  .detail {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .row.failed .detail {
    color: var(--danger);
  }

  .when {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
