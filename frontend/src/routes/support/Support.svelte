<script lang="ts">
  import { LifeBuoy } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { msg } from '../../lib/i18n';
  import { openSupportSite } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { parseTicketNumber, parseTopic } from '../../lib/support/topics';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import NewTicket from './NewTicket.svelte';
  import TicketList from './TicketList.svelte';
  import TicketView from './TicketView.svelte';

  let { ticket, view, topic }: { ticket?: string; view?: string; topic?: string } = $props();

  const number = $derived(parseTicketNumber(ticket));
  const needsAccount = $derived($authState === 'guest');

  async function signIn() {
    try {
      await leaveGuest();
    } catch (err) {
      toast(accountErrorText(err, msg('ui.signOutFailed')), 'danger');
    }
  }

  async function writeFromSite() {
    try {
      await openSupportSite();
    } catch (err) {
      console.error('open support page', err);
      toast(msg('tickets.siteOpenFailed'), 'danger');
    }
  }
</script>

{#if needsAccount}
  <Card surface="panel">
    <PageHeader title={msg('tickets.title')} subtitle={msg('tickets.subtitle')} />
    <EmptyState title={msg('tickets.guestTitle')} description={msg('tickets.guestText')}>
      {#snippet icon()}
        <LifeBuoy size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={signIn}>{msg('tickets.guestSignIn')}</Button>
        <Button onclick={writeFromSite}>{msg('tickets.cantSignIn')}</Button>
      {/snippet}
    </EmptyState>
  </Card>
{:else if number !== null}
  {#key number}
    <TicketView {number} />
  {/key}
{:else if view === 'new'}
  <NewTicket initialTopic={parseTopic(topic)} />
{:else}
  <TicketList />
{/if}
