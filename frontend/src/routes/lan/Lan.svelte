<script lang="ts">
  import { MonitorSmartphone, Share2, Wifi, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import { offerLabel, rejectedSummary, transferLabel } from '../../lib/lan/lanText';
  import { cancel, hashing, lanStats, offers, receive, shares, transfers, unshare } from '../../lib/stores/lan';
  import { bytesSize, formatCount, relativeDate } from '../../lib/utils/format';

  const preparing = $derived(
    [...$hashing.values()].filter((p) => !$shares.some((s) => s.gameId === p.gameId && s.enabled)),
  );

  const activeTransfers = $derived($transfers.filter((t) => t.status === 'receiving'));

  const rejectedNote = $derived(rejectedSummary($lanStats));

  function hashPct(processed: number, total: number) {
    return total > 0 ? (processed / total) * 100 : 0;
  }

  function transferPct(downloaded: number, total: number) {
    return total > 0 ? (downloaded / total) * 100 : 0;
  }
</script>

<Card surface="panel">
  <PageHeader title="Локальная сеть" subtitle="Передача игр напрямую другим компьютерам в вашей локальной сети." />

  <div class="stats">
    <div class="stat">
      <StatTile value={formatCount($lanStats.peersKnown)} label="Компьютеров рядом">
        {#snippet icon()}<MonitorSmartphone size="1.8rem" strokeWidth={1.8} />{/snippet}
      </StatTile>
    </div>
    <div class="stat">
      <StatTile value={formatCount($lanStats.offersKnown)} label="Доступно игр">
        {#snippet icon()}<Wifi size="1.8rem" strokeWidth={1.8} />{/snippet}
      </StatTile>
    </div>
    <div class="stat">
      <StatTile value={formatCount($lanStats.sharesActive)} label="Раздаю игр">
        {#snippet icon()}<Share2 size="1.8rem" strokeWidth={1.8} />{/snippet}
      </StatTile>
    </div>
  </div>

  <section class="section">
    <h2>Доступно рядом <span class="count">{$offers.length}</span></h2>
    {#if $offers.length === 0}
      <EmptyState
        title="Ничего не найдено"
        description="Не видно других компьютеров? Убедитесь, что раздача включена и на другом компьютере, что сеть отмечена как «частная», и разрешите Typhon доступ, если Windows спросит про брандмауэр при первом запуске."
      >
        {#snippet icon()}
          <Wifi size="2rem" strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
      {#if rejectedNote}
        <p class="muted">Отклонено: {rejectedNote}</p>
      {/if}
    {:else}
      <div class="rows">
        {#each $offers as offer (offer.peerId + offer.infoHash)}
          <div class="row">
            <span class="row-title">{offerLabel(offer)}</span>
            <div class="row-actions">
              <Button variant="primary" size="sm" onclick={() => receive(offer.infoHash, offer.peerId)}>Скачать</Button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </section>

  <section class="section">
    <h2>Я раздаю <span class="count">{$shares.length}</span></h2>
    {#if $shares.length === 0 && preparing.length === 0}
      <p class="muted">Вы пока ничего не раздаёте.</p>
    {:else}
      <div class="rows">
        {#each preparing as progress (progress.gameId)}
          <div class="row">
            <div class="row-text">
              <span class="row-title">Готовится к раздаче… {Math.round(hashPct(progress.processedBytes, progress.totalBytes))}%</span>
              <ProgressBar value={hashPct(progress.processedBytes, progress.totalBytes)} height={4} />
            </div>
          </div>
        {/each}
        {#each $shares as share (share.gameId)}
          <div class="row">
            <span class="row-title">{share.title}</span>
            <span class="row-size">{bytesSize(share.sizeBytes)}</span>
            <span class="row-date">{relativeDate(share.builtAt)}</span>
            <div class="row-actions">
              <Button size="sm" onclick={() => unshare(share.gameId)}>Перестать раздавать</Button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </section>

  <section class="section">
    <h2>Передачи <span class="count">{activeTransfers.length}</span></h2>
    {#if activeTransfers.length === 0}
      <p class="muted">Нет активных передач.</p>
    {:else}
      <div class="rows">
        {#each activeTransfers as transfer (transfer.id)}
          <div class="row">
            <div class="row-text">
              <span class="row-title">{transfer.title}</span>
              <ProgressBar value={transferPct(transfer.downloaded, transfer.total)} height={4} />
              <span class="row-sub">{transferLabel(transfer)}</span>
            </div>
            <div class="row-actions">
              <IconButton label="Отменить" size="sm" onclick={() => cancel(transfer.id)}>
                <X size="1.5rem" strokeWidth={1.8} />
              </IconButton>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </section>
</Card>

<style>
  .stats {
    display: flex;
    align-items: flex-start;
    margin-bottom: var(--space-8);
  }

  .stat {
    padding: 0 var(--space-6);
    border-left: 1px solid var(--border);
  }

  .stat:first-child {
    padding-left: 0;
    border-left: 0;
  }

  .section {
    margin-bottom: var(--space-10);
    max-width: 140rem;
  }

  .section:last-child {
    margin-bottom: 0;
  }

  .section h2 {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    font-size: var(--font-xl);
    margin-bottom: var(--space-4);
  }

  .count {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: 5.2rem;
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .row-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .row-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .row-size,
  .row-date {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    text-align: right;
  }

  .row-size {
    min-width: 7rem;
  }

  .row-date {
    min-width: 9rem;
  }

  .row-actions {
    display: flex;
    gap: 2px;
    margin-left: var(--space-2);
  }

  @media (max-width: 1300px) {
    .row-date {
      display: none;
    }
  }
</style>
