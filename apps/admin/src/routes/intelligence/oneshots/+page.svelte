<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Check, Trash2, ExternalLink, RefreshCw } from 'lucide-svelte';

	// Pending oneshot queue — public URL submissions whose categories need LLM
	// extraction (structured entity types). They land as disabled extract
	// channels on the shared "User Oneshot Submissions" source; staff approve
	// (enable + crawl) or reject (delete) here. Curate-pipeline submissions
	// fire instantly and never appear in this list.

	const ONESHOT_SLUG = 'user-oneshot-submissions';

	interface PendingChannel {
		id: string;
		entry_url: string;
		pipeline: string;
		enabled: boolean;
		entity_types?: string[];
		created_at?: string;
	}

	let sourceID = $state('');
	let channels = $state<PendingChannel[]>([]);
	let loading = $state(true);
	let error = $state('');
	let acting = $state<Set<string>>(new Set());
	let rejecting = $state<PendingChannel | null>(null);

	const canList = $derived(hasPermission('intel.channels.list'));
	const canApprove = $derived(
		hasPermission('intel.channels.update') && hasPermission('intel.channels.crawl')
	);
	const canReject = $derived(hasPermission('intel.channels.delete'));

	async function load() {
		loading = true;
		error = '';
		try {
			if (!sourceID) {
				const src = await api<{ data: { id: string } }>(`/sources/${ONESHOT_SLUG}`);
				if (!src.ok) {
					error = src.error ?? 'Shared submissions source not found';
					return;
				}
				if (!src.data?.data?.id) {
					error = 'Shared submissions source not found';
					return;
				}
				sourceID = src.data.data.id;
			}
			const res = await api<{ data: PendingChannel[] }>(
				`/admin/intel/sources/${sourceID}/channels?enabled=false&pipeline=extract`
			);
			if (!res.ok) {
				error = res.error ?? 'Failed to load pending submissions';
				return;
			}
			channels = res.data?.data ?? [];
		} finally {
			loading = false;
		}
	}

	async function approve(ch: PendingChannel) {
		acting = new Set(acting).add(ch.id);
		error = '';
		try {
			const en = await api(`/admin/intel/channels/${ch.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled: true })
			});
			if (!en.ok) {
				error = en.error ?? 'Failed to enable channel';
				return;
			}
			const crawl = await api(`/admin/intel/channels/${ch.id}/crawl`, { method: 'POST' });
			if (!crawl.ok) {
				error = crawl.error ?? 'Enabled but crawl enqueue failed';
				return;
			}
			channels = channels.filter((c) => c.id !== ch.id);
		} finally {
			acting = new Set([...acting].filter((id) => id !== ch.id));
		}
	}

	async function confirmReject() {
		if (!rejecting) return;
		const ch = rejecting;
		rejecting = null;
		acting = new Set(acting).add(ch.id);
		error = '';
		try {
			const res = await api(`/admin/intel/channels/${ch.id}`, { method: 'DELETE' });
			if (!res.ok) {
				error = res.error ?? 'Failed to delete channel';
				return;
			}
			channels = channels.filter((c) => c.id !== ch.id);
		} finally {
			acting = new Set([...acting].filter((id) => id !== ch.id));
		}
	}

	function fmtDate(iso?: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('nav.intel_oneshots', 'Source Submissions')} — {t('app.name', 'JapaConnect Admin')}</title>
</svelte:head>

{#if !canList}
	<div class="p-8 text-sm" style="color: var(--color-muted)">
		{t('common.no_permission', 'You do not have permission to view source submissions.')}
	</div>
{:else}
	<div class="mx-auto max-w-5xl p-6">
		<div class="mb-6 flex items-start justify-between gap-4">
			<div>
				<h1 class="text-xl font-semibold">{t('intel.oneshots_title', 'Source Submissions')}</h1>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{t(
						'intel.oneshots_desc',
						'Member-submitted URLs claiming structured content (scholarships, jobs, visas, …). Approving runs LLM extraction — only approve trusted sources.'
					)}
				</p>
			</div>
			<Button variant="outline" size="sm" onclick={load} disabled={loading}>
				<RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
				{t('common.refresh', 'Refresh')}
			</Button>
		</div>

		{#if error}
			<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
				{error}
			</div>
		{/if}

		{#if loading && channels.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t('common.loading', 'Loading…')}
			</div>
		{:else if channels.length === 0}
			<div class="rounded-md border py-12 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
				{t('intel.oneshots_empty', 'No pending submissions — the queue is clear.')}
			</div>
		{:else}
			<div class="overflow-x-auto rounded-md border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b text-left" style="border-color: var(--color-border); color: var(--color-muted)">
							<th class="px-4 py-3 font-medium">{t('intel.col_url', 'URL')}</th>
							<th class="px-4 py-3 font-medium">{t('intel.col_types', 'Entity types')}</th>
							<th class="px-4 py-3 font-medium">{t('intel.col_submitted', 'Submitted')}</th>
							<th class="px-4 py-3 font-medium text-right">{t('intel.col_actions', 'Actions')}</th>
						</tr>
					</thead>
					<tbody>
						{#each channels as ch (ch.id)}
							<tr class="border-b last:border-0" style="border-color: var(--color-border)">
								<td class="max-w-md px-4 py-3">
									<a href={ch.entry_url} target="_blank" rel="noopener noreferrer"
										class="inline-flex items-center gap-1 break-all hover:underline" style="color: var(--color-accent)">
										{ch.entry_url}
										<ExternalLink size={12} />
									</a>
								</td>
								<td class="px-4 py-3">
									{#each ch.entity_types ?? [] as et}
										<span class="mr-1 inline-block rounded px-1.5 py-0.5 text-xs" style="background: var(--color-surface-2); color: var(--color-muted)">{et}</span>
									{/each}
								</td>
								<td class="whitespace-nowrap px-4 py-3" style="color: var(--color-muted)">
									{fmtDate(ch.created_at)}
								</td>
								<td class="px-4 py-3">
									<div class="flex justify-end gap-2">
										{#if canApprove}
											<Button size="sm" onclick={() => approve(ch)} disabled={acting.has(ch.id)}>
												<Check size={14} />
												{acting.has(ch.id) ? t('intel.approving', 'Approving…') : t('intel.approve_crawl', 'Approve & Crawl')}
											</Button>
										{/if}
										{#if canReject}
											<Button size="sm" variant="outline" onclick={() => (rejecting = ch)} disabled={acting.has(ch.id)}>
												<Trash2 size={14} />
												{t('intel.reject', 'Reject')}
											</Button>
										{/if}
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<Modal open={rejecting !== null} title={t('intel.reject_title', 'Reject submission?')} onclose={() => (rejecting = null)}>
		<p class="text-sm" style="color: var(--color-muted)">
			{t('intel.reject_desc', 'The channel will be deleted and the URL will not be fetched or extracted.')}
		</p>
		{#if rejecting}
			<p class="mt-2 break-all text-sm">{rejecting.entry_url}</p>
		{/if}
		{#snippet footer()}
			<Button variant="outline" onclick={() => (rejecting = null)}>{t('common.cancel', 'Cancel')}</Button>
			<Button variant="destructive" onclick={confirmReject}>{t('intel.reject_confirm', 'Reject & Delete')}</Button>
		{/snippet}
	</Modal>
{/if}
