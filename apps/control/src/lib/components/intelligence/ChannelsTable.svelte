<script lang="ts">
	import Modal from '$lib/components/ui/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import ChannelEditor from '$lib/components/intelligence/ChannelEditor.svelte';
	import type { ChannelSubmitValues } from '$lib/components/intelligence/ChannelEditor.svelte';
	import { Plus, Pencil, Trash2, Zap, RefreshCw, Radar, Loader2, Rss } from 'lucide-svelte';

	// ChannelsTable is the per-source channels editor (intel-channels Phase
	// 1, spec §5): a dialog listing every intel_source_channels row with
	// enable toggles, crawl-now, edit, delete, an add-channel form, and the
	// source-level Re-probe action. Opened from the sources registry row
	// action; the parent passes the meta option lists so every select stays
	// backend-driven (Rule 16).

	// IntelChannel mirrors the admin channel entry returned by
	// GET /api/v1/admin/intel/sources/{id}/channels.
	export interface IntelChannel {
		id: string;
		source_id: string;
		label: string;
		entry_url: string;
		crawl_method: string;
		entity_types: string[];
		pipeline: string;
		provider_config?: Record<string, unknown> | null;
		crawl_frequency_mins: number;
		enabled: boolean;
		origin: string;
		relevance_mode: string;
		relevance_topics: string[];
		relevance_url_patterns: string[];
		consecutive_failures?: number;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		last_error_message?: string | null;
		created_at?: string;
		updated_at?: string;
	}

	interface SourceRef {
		id: string;
		name: string;
		enabled: boolean;
	}

	let {
		open = $bindable(false),
		source = null as SourceRef | null,
		entityTypeOptions = [] as string[],
		crawlMethodOptions = [] as string[],
		pipelineOptions = [] as string[]
	}: {
		open: boolean;
		source?: SourceRef | null;
		entityTypeOptions?: string[];
		crawlMethodOptions?: string[];
		pipelineOptions?: string[];
	} = $props();

	// Per-action RBAC gates — each admin channel route carries its own
	// intel.channels.* node (migration 000684); reprobe lives under
	// intel.sources.probe.
	const canList = $derived(hasAnyPermission(['intel.channels.list', 'intel.channels.*']));
	const canCreate = $derived(hasAnyPermission(['intel.channels.create', 'intel.channels.*']));
	const canUpdate = $derived(hasAnyPermission(['intel.channels.update', 'intel.channels.*']));
	const canDelete = $derived(hasAnyPermission(['intel.channels.delete', 'intel.channels.*']));
	const canCrawl = $derived(hasAnyPermission(['intel.channels.crawl', 'intel.channels.*']));
	const canReprobe = $derived(hasAnyPermission(['intel.sources.probe', 'intel.sources.*']));

	let channels = $state<IntelChannel[]>([]);
	let loading = $state(false);
	let errorMessage = $state('');
	let notice = $state('');

	let editorOpen = $state(false);
	let editingChannel = $state<IntelChannel | null>(null);
	let editorSaving = $state(false);
	let editorError = $state('');

	let deleteTarget = $state<IntelChannel | null>(null);
	let deleteOpen = $state(false);

	let togglingId = $state<string | null>(null);
	let crawlingId = $state<string | null>(null);
	let reprobing = $state(false);

	async function load() {
		if (!source || !canList) return;
		loading = true;
		errorMessage = '';
		const result = await api<IntelChannel[]>(
			`/api/v1/admin/intel/sources/${source.id}/channels`
		);
		if (result.ok) {
			channels = Array.isArray(result.data) ? result.data : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	// Load channels each time the dialog opens; clear state on close.
	$effect(() => {
		if (open && source?.id) {
			notice = '';
			load();
		}
	});

	function formatTime(ts?: string | null): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	function sameArr(a: string[], b: string[]): boolean {
		if (a.length !== b.length) return false;
		const bs = new Set(b);
		return a.every((v) => bs.has(v));
	}

	function openCreate() {
		editingChannel = null;
		editorError = '';
		editorOpen = true;
	}

	function openEdit(ch: IntelChannel) {
		editingChannel = ch;
		editorError = '';
		editorOpen = true;
	}

	async function handleSubmit(values: ChannelSubmitValues) {
		if (!source) return;
		editorSaving = true;
		editorError = '';
		let result: { ok: boolean; error?: string };
		if (editingChannel?.id) {
			// PATCH presence semantics: send only changed fields. entity_types
			// and relevance_* are explicit-set keys — present (even empty)
			// means "set to this", absent means "keep current".
			const orig = editingChannel;
			const body: Record<string, unknown> = {};
			if (values.label !== orig.label) body.label = values.label;
			if (values.entry_url !== orig.entry_url) body.entry_url = values.entry_url;
			if (values.crawl_method !== orig.crawl_method) body.crawl_method = values.crawl_method;
			if (values.pipeline !== orig.pipeline) body.pipeline = values.pipeline;
			if (values.crawl_frequency_mins !== orig.crawl_frequency_mins)
				body.crawl_frequency_mins = values.crawl_frequency_mins;
			if (values.enabled !== orig.enabled) body.enabled = values.enabled;
			if (!sameArr(values.entity_types, orig.entity_types ?? [])) body.entity_types = values.entity_types;
			const origConfig = orig.provider_config ?? {};
			const newConfig = values.provider_config ?? {};
			if (JSON.stringify(origConfig) !== JSON.stringify(newConfig)) body.provider_config = newConfig;
			if (values.relevance_mode !== orig.relevance_mode) body.relevance_mode = values.relevance_mode;
			if (!sameArr(values.relevance_topics, orig.relevance_topics ?? []))
				body.relevance_topics = values.relevance_topics;
			if (!sameArr(values.relevance_url_patterns, orig.relevance_url_patterns ?? []))
				body.relevance_url_patterns = values.relevance_url_patterns;

			if (Object.keys(body).length === 0) {
				editorSaving = false;
				editorOpen = false;
				return;
			}
			result = await api(`/api/v1/admin/intel/channels/${orig.id}`, {
				method: 'PATCH',
				body
			});
		} else {
			const body: Record<string, unknown> = {
				label: values.label,
				entry_url: values.entry_url,
				crawl_method: values.crawl_method,
				pipeline: values.pipeline,
				entity_types: values.entity_types,
				crawl_frequency_mins: values.crawl_frequency_mins,
				enabled: values.enabled,
				relevance_mode: values.relevance_mode,
				relevance_topics: values.relevance_topics,
				relevance_url_patterns: values.relevance_url_patterns
			};
			if (values.provider_config !== undefined) body.provider_config = values.provider_config;
			result = await api(`/api/v1/admin/intel/sources/${source.id}/channels`, {
				method: 'POST',
				body
			});
		}
		editorSaving = false;
		if (result.ok) {
			editorOpen = false;
			await load();
		} else {
			editorError = result.error ?? t('intel.channel_save_failed', 'Failed to save channel');
		}
	}

	async function toggleEnabled(ch: IntelChannel) {
		togglingId = ch.id;
		errorMessage = '';
		const result = await api(`/api/v1/admin/intel/channels/${ch.id}`, {
			method: 'PATCH',
			body: { enabled: !ch.enabled }
		});
		togglingId = null;
		if (result.ok) {
			await load();
		} else {
			errorMessage = result.error;
		}
	}

	// Crawl-now enqueues intel:fetch_channel — the backend rejects disabled
	// channels and inbound/manual methods with 409, surfaced as the error.
	async function crawlNow(ch: IntelChannel) {
		crawlingId = ch.id;
		errorMessage = '';
		notice = '';
		const result = await api(`/api/v1/admin/intel/channels/${ch.id}/crawl`, { method: 'POST' });
		crawlingId = null;
		if (result.ok) {
			notice = t('intel.crawl_queued', 'Crawl queued for this channel.');
		} else {
			errorMessage = result.error;
		}
	}

	// Re-probe enqueues intel:probe_verify — re-runs detection and
	// re-materializes channels idempotently (probe-origin rows dedupe on
	// source+entry_url+entity_types).
	async function reprobe() {
		if (!source) return;
		reprobing = true;
		errorMessage = '';
		notice = '';
		const result = await api(`/api/v1/admin/intel/sources/${source.id}/reprobe`, { method: 'POST' });
		reprobing = false;
		if (result.ok) {
			notice = t('intel.reprobe_queued', 'Re-probe queued — detected channels refresh when the probe completes.');
		} else {
			errorMessage = result.error;
		}
	}

	function openDelete(ch: IntelChannel) {
		deleteTarget = ch;
		deleteOpen = true;
	}

	async function confirmDeleteChannel() {
		if (!deleteTarget) return;
		const result = await api(`/api/v1/admin/intel/channels/${deleteTarget.id}`, { method: 'DELETE' });
		if (result.ok) {
			deleteTarget = null;
			await load();
		} else {
			errorMessage = result.error;
		}
	}
</script>

<Modal bind:open title={t('intel.channels_title', 'Channels')} size="max-w-4xl" hideHeader>
	{#snippet children(close)}
		<div class="flex h-full flex-col">
			<div class="flex items-start justify-between gap-4">
				<div>
					<h2 class="text-foreground text-lg font-semibold">
						{t('intel.channels_title', 'Channels')}
					</h2>
					<p class="text-muted-foreground mt-1 text-sm">
						{source?.name ?? ''} — {t('intel.channels_desc', 'Entry points (URL + method + pipeline) the crawler fetches for this source.')}
					</p>
				</div>
				<div class="flex shrink-0 items-center gap-2">
					{#if canReprobe}
						<button
							class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
							disabled={reprobing || !source?.enabled}
							title={source?.enabled ? '' : t('intel.reprobe_disabled_hint', 'Enable the source to re-probe')}
							onclick={reprobe}
						>
							{#if reprobing}
								<Loader2 class="size-4 animate-spin" />
							{:else}
								<Radar class="size-4" />
							{/if}
							{t('intel.reprobe', 'Re-probe')}
						</button>
					{/if}
					<button
						class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
						disabled={loading}
						onclick={load}
					>
						<RefreshCw class="size-4 {loading ? 'animate-spin' : ''}" />
						{t('common.refresh', 'Refresh')}
					</button>
					{#if canCreate}
						<button
							class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-sm font-medium transition-colors hover:opacity-90"
							onclick={openCreate}
						>
							<Plus class="size-4" />
							{t('intel.add_channel', 'Add channel')}
						</button>
					{/if}
				</div>
			</div>

			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}
			{#if notice}
				<div class="mt-3 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600">
					{notice}
				</div>
			{/if}

			<div class="border-border-card mt-4 flex-1 overflow-y-auto rounded-lg border">
				{#if !canList}
					<p class="text-muted-foreground px-4 py-8 text-center text-sm">
						{t('intel.channels_access_denied', 'You need intel.channels.list permission to view channels.')}
					</p>
				{:else if loading && channels.length === 0}
					<div class="space-y-2 p-4">
						{#each Array(3) as _}
							<div class="bg-muted h-10 animate-pulse rounded"></div>
						{/each}
					</div>
				{:else if channels.length === 0}
					<div class="flex flex-col items-center gap-2 px-4 py-12">
						<Rss class="text-muted-foreground size-8 opacity-30" />
						<p class="text-muted-foreground text-sm">
							{t('intel.no_channels', 'No channels yet — add one or re-probe the source to auto-detect them.')}
						</p>
					</div>
				{:else}
					<table class="w-full text-[13px]">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.channel_label', 'Label')}</th>
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.field_crawl_method', 'Method')}</th>
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.channel_pipeline', 'Pipeline')}</th>
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.field_entity_types', 'Entity types')}</th>
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.last_status', 'Last status')}</th>
								<th class="text-muted-foreground bg-muted px-3 py-2 text-start font-medium">{t('intel.field_enabled', 'Enabled')}</th>
								<th class="text-muted-foreground bg-muted w-24 px-3 py-2"></th>
							</tr>
						</thead>
						<tbody>
							{#each channels as ch (ch.id)}
								<tr class="border-border-card border-t">
									<td class="px-3 py-2.5">
										<div class="flex flex-col">
											<span class="font-medium">{ch.label}</span>
											<span class="text-muted-foreground max-w-52 truncate font-mono text-xs" title={ch.entry_url}>{ch.entry_url}</span>
										</div>
									</td>
									<td class="px-3 py-2.5">
										<span class="bg-muted inline-flex items-center rounded-full px-2 py-0.5 font-mono text-xs uppercase">
											{ch.crawl_method}
										</span>
									</td>
									<td class="px-3 py-2.5">
										<span class="inline-flex items-center rounded-full bg-sky-500/10 px-2 py-0.5 text-xs font-medium text-sky-600">
											{ch.pipeline}
										</span>
									</td>
									<td class="px-3 py-2.5">
										{#if ch.entity_types?.length}
											<div class="flex max-w-40 flex-wrap gap-1">
												{#each ch.entity_types as et}
													<span class="bg-muted rounded-full px-2 py-0.5 text-xs">{et}</span>
												{/each}
											</div>
										{:else}
											<span class="text-muted-foreground text-xs">—</span>
										{/if}
									</td>
									<td class="px-3 py-2.5">
										{#if ch.last_error_message}
											<div class="flex flex-col">
												<span class="text-destructive text-xs" title={ch.last_error_message}>
													{ch.last_error_message.length > 60 ? ch.last_error_message.slice(0, 60) + '…' : ch.last_error_message}
												</span>
												<span class="text-muted-foreground text-xs">
													{formatTime(ch.last_error_at)}{(ch.consecutive_failures ?? 0) > 1 ? ` · ×${ch.consecutive_failures}` : ''}
												</span>
											</div>
										{:else}
											<span class="text-muted-foreground text-xs">{formatTime(ch.last_success_at ?? ch.last_crawled_at)}</span>
										{/if}
									</td>
									<td class="px-3 py-2.5">
										{#if canUpdate}
											<label class="flex cursor-pointer items-center gap-1.5 text-xs" class:opacity-50={togglingId === ch.id}>
												<input
													type="checkbox"
													checked={ch.enabled}
													disabled={togglingId === ch.id}
													onchange={() => toggleEnabled(ch)}
													class="border-border-input bg-background size-3.5 rounded-sm accent-accent-foreground"
												/>
												{ch.enabled ? t('intel.enabled', 'Enabled') : t('intel.disabled', 'Disabled')}
											</label>
										{:else}
											<span class="text-xs {ch.enabled ? 'text-emerald-500' : 'text-muted-foreground'}">
												{ch.enabled ? t('intel.enabled', 'Enabled') : t('intel.disabled', 'Disabled')}
											</span>
										{/if}
									</td>
									<td class="px-3 py-2.5">
										<div class="flex items-center justify-end gap-1">
											{#if canCrawl}
												<button
													class="text-muted-foreground hover:bg-dark-04 hover:text-foreground flex h-7 w-7 items-center justify-center rounded-lg transition-colors disabled:opacity-40"
													title={t('intel.crawl_now', 'Crawl Now')}
													disabled={crawlingId === ch.id || !ch.enabled}
													onclick={() => crawlNow(ch)}
												>
													{#if crawlingId === ch.id}
														<Loader2 class="size-3.5 animate-spin" />
													{:else}
														<Zap class="size-3.5" />
													{/if}
												</button>
											{/if}
											{#if canUpdate}
												<button
													class="text-muted-foreground hover:bg-dark-04 hover:text-foreground flex h-7 w-7 items-center justify-center rounded-lg transition-colors"
													title={t('intel.edit', 'Edit')}
													onclick={() => openEdit(ch)}
												>
													<Pencil class="size-3.5" />
												</button>
											{/if}
											{#if canDelete}
												<button
													class="text-muted-foreground hover:bg-dark-04 hover:text-destructive flex h-7 w-7 items-center justify-center rounded-lg transition-colors"
													title={t('common.delete', 'Delete')}
													onclick={() => openDelete(ch)}
												>
													<Trash2 class="size-3.5" />
												</button>
											{/if}
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>

			<div class="mt-4 flex items-center justify-between">
				<p class="text-muted-foreground text-xs">
					{#if channels.some((c) => c.origin === 'probe')}
						{t('intel.probe_channels_note', 'Channels marked by the probe re-materialize on re-probe; admin edits are preserved.')}
					{/if}
				</p>
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('common.close', 'Close')}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>

<ChannelEditor
	bind:open={editorOpen}
	channel={editingChannel}
	{entityTypeOptions}
	{crawlMethodOptions}
	{pipelineOptions}
	saving={editorSaving}
	errorMessage={editorError}
	onSubmit={handleSubmit}
/>

<ConfirmModal
	bind:open={deleteOpen}
	title={t('intel.delete_channel', 'Delete Channel')}
	consequence={deleteTarget
		? t('intel.delete_channel_consequence', 'This removes the channel entry point. Already-ingested records are preserved.')
		: ''}
	confirmText={deleteTarget?.label ?? ''}
	onConfirm={confirmDeleteChannel}
/>
