<script lang="ts">
  import { ChevronDown, Gauge } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Popover from '../../lib/components/Popover.svelte';
  import Reading from '../../lib/components/Reading.svelte';
  import Slider from '../../lib/components/Slider.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { navigate } from '../../lib/stores/router';
  import { updateSettingsReporting } from '../../lib/stores/settings';
  import { codeText, locale, t } from '../../lib/i18n';
  import { makeFormat } from '../../lib/utils/format';
  import { SPEED_LIMIT_NO_LIMIT, SPEED_LIMIT_STEPS_MB, speedLimitAt, speedLimitPosition } from '../../lib/utils/speedLimit';

  /**
   * The download speed limit from the page's header: a button saying the stored limit opens a slider
   * over the limit's steps. The value shown is the setting (`limit`, bytes/s, 0 for none), or the step
   * under a dragging pointer; a key or a release saves through the settings store, which shows the new
   * value at once and puts the stored one back when the save is refused. Finer values, the upload limit
   * and the limit while a game runs stay in Settings, which the popover leads to.
   */
  let { limit }: { limit: number } = $props();

  const MB = 1024 ** 2;
  const fmt = $derived(makeFormat($locale, $t));

  let open = $state(false);
  let draft = $state<number | null>(null);
  let saving = $state(false);
  let error = $state('');

  const shownLimit = $derived(draft === null ? limit : speedLimitAt(draft));
  const position = $derived(draft ?? speedLimitPosition(limit));
  const buttonText = $derived(
    $t('downloads.page.limitButton', { value: shownLimit > 0 ? fmt.rateLimitLabel(shownLimit) : $t('downloads.page.limitNone') }),
  );

  $effect(() => {
    if (open) error = '';
  });

  async function commit(at: number) {
    const next = speedLimitAt(at);
    if (next === limit) return;
    error = '';
    saving = true;
    await updateSettingsReporting({ downloadRateLimit: next }, (err) => {
      const reason = codeText(err, '');
      error = reason ? $t('downloads.page.limitSaveFailed', { reason }) : $t('downloads.page.limitSaveFailedPlain');
    });
    saving = false;
  }
</script>

<Popover bind:open label={$t('downloads.page.limitLabel')} placement="bottom-end" width="32rem">
  {#snippet trigger({ props })}
    <Button {...props}>
      <Gauge size="1.5rem" strokeWidth={1.8} />
      <Reading text={buttonText} />
      <ChevronDown size="1.4rem" strokeWidth={1.8} />
    </Button>
  {/snippet}
  {#snippet children({ close })}
    <div class="limit">
      <Slider
        {position}
        max={SPEED_LIMIT_NO_LIMIT}
        label={$t('downloads.page.limitLabel')}
        valueText={fmt.rateLimitLabel(shownLimit)}
        startText={fmt.rateLimitLabel(SPEED_LIMIT_STEPS_MB[0] * MB)}
        endText={fmt.rateLimitLabel(0)}
        busy={saving}
        oninput={(at) => (draft = at)}
        onchange={commit}
      />
      {#if error}
        <Notice tone="danger" compact>{error}</Notice>
      {/if}
      <p class="finer">
        {$t('downloads.page.limitFiner')}
        <TextLink
          onclick={() => {
            close();
            navigate('settings', { tab: 'downloads' });
          }}>{$t('downloads.page.limitSettings')}</TextLink
        >
      </p>
    </div>
  {/snippet}
</Popover>

<style>
  .limit {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3) var(--space-3);
  }

  .finer {
    font-size: var(--font-xs);
    color: var(--text-2);
  }
</style>
