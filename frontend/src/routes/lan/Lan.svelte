<script lang="ts">
  import { Wifi, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import { offerLabel, rejectedSummary, transferLabel } from '../../lib/lan/lanText';
  import { cancel, hashing, lanStats, offers, receive, shares, transfers, unshare } from '../../lib/stores/lan';
  import { bytesSize, relativeDate } from '../../lib/utils/format';

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

<PageHeader title="Локальная сеть" subtitle="Передача игр напрямую другим компьютерам в вашей локальной сети." />

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

<style>
  .section {
    margin-bottom: var(--space-10);
    max-width: 140rem;
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
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: 5.2rem;
    padding: 0.6rem 1.2rem;
    margin: 0 -1.2rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .row + .row {
    border-top: 1px solid var(--border);
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
