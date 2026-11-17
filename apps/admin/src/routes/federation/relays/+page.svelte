<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /federation/relays — §126 M3 relay subscriptions (FEP-ae0c). EntityPage scaffold; create/delete only, relays are immutable.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { federationAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { Eye, Trash2 } from 'lucide-svelte';

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pendingUnsub = $state<Relay | null>(null);
	let showUnsubConfirm = $state(false);
	let unsubscribing = $state(false);
	let unsubError = $state('');

	async function confirmUnsubscribe() {
		if (!pendingUnsub) return;
		unsubscribing = true;
		unsubError = '';
		const res = await federationAdminApi.deleteRelay(pendingUnsub.id);
		unsubscribing = false;
		if (!res.ok) {
			unsubError = res.error ?? t('common.error', 'Unsubscribe failed');
			return;
		}
		showUnsubConfirm = false;
		pendingUnsub = null;
		entityController?.reload();
	}

	type Relay = {
		id: string;
		actor_url: string;
		inbox_url: string;
		domain: string;
		status: string;
		follow_ap_id?: string | null;
		last_error?: string | null;
		created_at: string;
	};

	const statusOptions = [
		{ value: 'pending', label: t('federation.relays.status_pending', 'Pending') },
		{ value: 'accepted', label: t('federation.relays.status_accepted', 'Accepted') },
		{ value: 'rejected', label: t('federation.relays.status_rejected', 'Rejected') },
		{ value: 'failed', label: t('federation.relays.status_failed', 'Failed') },
		{ value: 'disabled', label: t('federation.relays.status_disabled', 'Disabled') }
	];

	const fields: FieldDef[] = [
		{
			key: 'actor_url',
			label: t('federation.relays.actor_url', 'Relay actor URL'),
			type: 'url',
			placeholder: 'https://relay.example/actor',
			required: true,
			help: t('federation.relays.actor_url_hint', 'The relay\u2019s Application or Service actor URL. Subscribing sends Follow{Public} from the instance actor.')
		}
	];

	const config: EntityPageConfig = {
		title: t('federation.relays.title', 'Federation Relays'),
		description: t('federation.relays.description', 'Relay subscriptions expanding the instance\u2019s federation reach (FEP-ae0c).'),
		createLabel: t('federation.relays.create', 'Subscribe Relay'),
		createPermission: 'federation.instances.manage',
		deletePermission: 'federation.instances.manage',
		pageSize: 25,
		columns: [
			{
				key: 'actor_url',
				label: t('federation.relays.actor_url', 'Relay actor URL'),
				render: (v: string) => `<span class="font-mono text-xs break-all" style="color: var(--color-foreground)">${v}</span>`
			},
			{ key: 'domain', label: t('federation.relays.domain', 'Domain') },
			{
				key: 'inbox_url',
				label: t('federation.relays.inbox_url', 'Inbox'),
				render: (v: string) => `<span class="font-mono text-xs truncate" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'status', label: t('federation.relays.status', 'Status'), componentSnippet: StatusCell },
			{
				key: 'last_error',
				label: t('federation.relays.last_error', 'Last error'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-danger)">${v ?? ''}</span>`,
				defaultHidden: true
			},
			{ key: 'created_at', label: t('common.created_at', 'Created'), componentSnippet: DateCell }
		],
		filters: [
			{
				key: 'status',
				label: t('federation.relays.status', 'Status'),
				options: statusOptions
			}
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row, ctx) => ctx?.openView(row)
			},
			{
				label: t('federation.relays.unsubscribe', 'Unsubscribe'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Relay) => {
					pendingUnsub = row;
					unsubError = '';
					showUnsubConfirm = true;
				}
			}
		],
		fields,
		// Unsubscribe goes through the page-local confirm modal — the §126
		// copy must note the remote relay is sent Undo{Follow}.
		bulkDelete: false,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page: String(page + 1), // backend pages are 1-based
				limit: String(pageSize)
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			const result = await federationAdminApi.listRelays(params.toString());
			if (result.ok) {
				const d = result.data as { data?: Relay[]; meta?: { total?: number; total_pages?: number } };
				return { data: d.data ?? [], total: d.meta?.total ?? 0, totalPages: d.meta?.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load relays'));
		},
		create: async (body) => {
			const res = await federationAdminApi.createRelay({ actor_url: String(body.actor_url ?? '').trim() });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: RelayView
	};
</script>

<svelte:head>
	<title>{t('federation.relays.title', 'Federation Relays')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />

<!-- Unsubscribe confirm — §126: copy notes the Undo{Follow} send. -->
<Modal bind:open={showUnsubConfirm} title={t('federation.relays.unsubscribe', 'Unsubscribe')}>
	{#if pendingUnsub}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('federation.relays.unsubscribe_confirm', 'Unsubscribe from this relay? The remote relay is notified via an Undo{Follow} from the instance actor and its inbox stops receiving our public activities.')}
		</p>
		<p class="mt-2 break-all font-mono text-xs" style="color: var(--color-muted)">{pendingUnsub.actor_url}</p>
	{/if}
	{#if unsubError}
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{unsubError}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showUnsubConfirm = false)} disabled={unsubscribing}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmUnsubscribe} disabled={unsubscribing}>
			{unsubscribing ? t('common.working', 'Working…') : t('federation.relays.unsubscribe', 'Unsubscribe')}
		</Button>
	</div>
</Modal>

{#snippet StatusCell({ value }: { value: any; row: Relay })}
	{@const tones: Record<string, string> = {
		accepted: 'bg-success/15 text-success',
		pending: 'bg-warning/15 text-warning',
		rejected: 'bg-danger/15 text-danger',
		failed: 'bg-danger/15 text-danger',
		disabled: 'bg-muted/15 text-muted'
	}}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {tones[String(value)] ?? 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}

{#snippet DateCell({ value }: { value: any; row: Relay })}
	<span class="text-xs" style="color: var(--color-muted)">{value ? new Date(String(value)).toLocaleString() : '—'}</span>
{/snippet}

{#snippet RelayView({ row }: { row: Relay })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.actor_url', 'Relay actor URL')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.actor_url}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.inbox_url', 'Inbox')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.inbox_url}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.domain', 'Domain')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.domain}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.status', 'Status')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.status}</p>
			</div>
		</div>
		{#if row.last_error}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.last_error', 'Last error')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-danger)">{row.last_error}</p>
			</div>
		{/if}
		{#if row.follow_ap_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.relays.follow_id', 'Follow activity')}</p>
				<p class="mt-1 break-all text-xs font-mono" style="color: var(--color-muted)">{row.follow_ap_id}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
