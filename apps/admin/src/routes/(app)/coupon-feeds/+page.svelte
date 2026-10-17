<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /coupon-feeds — deal feed source management (EntityPage scaffold).
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (Deal Feed Import)
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import type { ActionContext } from '$lib/components/ui/data-table';
	import { couponFeedsAdminApi, affiliateProvidersAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye, Pencil, RefreshCw, Trash2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	type FeedSource = {
		id: string;
		name: string;
		source_type: string;
		endpoint_url: string;
		has_api_key: boolean;
		default_provider_id: string | null;
		enabled: boolean;
		last_synced_at: string | null;
		last_status: string | null;
		last_error: string | null;
		stats: { seen?: number; created?: number; updated?: number; suspended?: number; expired?: number; skipped?: number } | null;
		coupon_count?: number;
		created_at: string;
		updated_at: string;
	};

	type Provider = {
		id: string;
		name: string;
		slug: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let pageNotice = $state('');
	let syncingId = $state<string | null>(null);
	let providerOptions = $state<Array<{ value: string; label: string }>>([]);

	// Load affiliate providers for the default_provider_id select (§55).
	onMount(async () => {
		try {
			const res = await affiliateProvidersAdminApi.list('page_size=100&is_active=true');
			if (res.ok && Array.isArray(res.data)) {
				providerOptions = (res.data as Provider[]).map((p) => ({ value: p.id, label: p.name }));
			}
		} catch {
			// Best-effort — the select is simply empty without it.
		}
	});

	const sourceTypeOptions = [
		{ value: 'couponapi', label: t('coupon_feeds.couponapi', 'CouponAPI.org (incremental)') },
		{ value: 'generic', label: t('coupon_feeds.generic', 'Generic JSON feed (full snapshot)') }
	];

	function fetchProviderOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? providerOptions
			: providerOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchProviderOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(providerOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getProviderSelectedLabel(id: string): string {
		return providerOptions.find((o) => o.value === id)?.label ?? id;
	}

	// api_key is write-only — never returned by the API; leaving it blank on
	// edit keeps the stored key (COALESCE in the update query).
	let fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('coupon_feeds.name', 'Name'), type: 'text', required: true, placeholder: 'e.g. CouponAPI.org' },
		{ key: 'source_type', label: t('coupon_feeds.source_type', 'Source Type'), type: 'select', options: sourceTypeOptions, required: true },
		{ key: 'endpoint_url', label: t('coupon_feeds.endpoint_url', 'Endpoint URL'), type: 'url', required: true, placeholder: 'https://couponapi.org/api/getIncrementalFeed/' },
		{ key: 'api_key', label: t('coupon_feeds.api_key', 'API Key'), type: 'text', placeholder: t('coupon_feeds.api_key_hint', 'Leave blank to keep the stored key') },
		{ key: 'default_provider_id', label: t('coupon_feeds.default_provider', 'Default Provider'), type: 'autocomplete', placeholder: t('coupon_feeds.default_provider_hint', 'Optional — else matched per offer store'), emptyMessage: t('common.no_results', 'No results found'), fetcher: fetchProviderOptions, initialFetcher: fetchProviderOptionsInitial, getSelectedLabel: getProviderSelectedLabel },
		{ key: 'enabled', label: t('coupon_feeds.enabled', 'Enabled'), type: 'checkbox' }
	]);

	async function syncNow(row: FeedSource, ctx?: ActionContext) {
		if (syncingId) return;
		syncingId = row.id;
		pageError = '';
		pageNotice = '';
		try {
			const res = await couponFeedsAdminApi.sync(row.id);
			if (res.ok) {
				pageNotice = t('coupon_feeds.sync_queued', 'Sync queued — refresh in a moment to see results.');
			} else {
				pageError = res.error ?? t('common.error', 'Failed to queue sync');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to queue sync');
		} finally {
			syncingId = null;
		}
		ctx?.reload();
	}

	function toPayload(body: Record<string, unknown>): Record<string, unknown> {
		const payload: Record<string, unknown> = {
			name: String(body.name ?? '').trim(),
			source_type: String(body.source_type ?? 'generic'),
			endpoint_url: String(body.endpoint_url ?? '').trim(),
			enabled: !!body.enabled
		};
		if (body.api_key) payload.api_key = String(body.api_key);
		payload.default_provider_id = body.default_provider_id ? String(body.default_provider_id) : null;
		return payload;
	}

	const config = $derived<EntityPageConfig>({
		title: t('coupon_feeds.title', 'Deal Feeds'),
		description: t('coupon_feeds.description', 'External deal/coupon feed sources — imported offers appear on /deal pages'),
		createLabel: t('coupon_feeds.create', 'Add Feed Source'),
		createPermission: 'coupons.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('coupon_feeds.name', 'Name'), sortable: true },
			{ key: 'source_type', label: t('coupon_feeds.source_type', 'Type'), sortable: true },
			{
				key: 'endpoint_url',
				label: t('coupon_feeds.endpoint_url', 'Endpoint'),
				render: (v: string) => (v ? (v.length > 50 ? v.slice(0, 50) + '…' : v) : '—')
			},
			{
				key: 'coupon_count',
				label: t('coupon_feeds.coupons', 'Coupons'),
				render: (v: number) => `${v ?? 0}`
			},
			{
				key: 'last_status',
				label: t('coupon_feeds.last_sync', 'Last Sync'),
				render: (v: string, row: FeedSource) =>
					row.last_synced_at
						? `${v ?? '—'} · ${new Date(row.last_synced_at).toLocaleString()}`
						: t('coupon_feeds.never_synced', 'Never')
			},
			{
				key: 'enabled',
				label: t('common.status', 'Status'),
				render: (v: boolean) => (v ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled'))
			}
		],
		filters: [
			{ key: 'source_type', label: t('coupon_feeds.source_type', 'Source Type'), options: sourceTypeOptions }
		],
		actions: [
			{ label: t('coupon_feeds.sync_now', 'Sync Now'), icon: RefreshCw, onclick: (row: FeedSource, ctx?: ActionContext) => syncNow(row, ctx), show: (row: FeedSource) => row.enabled && syncingId !== row.id },
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: FeedSource, ctx?: ActionContext) => ctx?.openView(row) },
			{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row: FeedSource, ctx?: ActionContext) => ctx?.openEdit(row) },
			{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row: FeedSource, ctx?: ActionContext) => ctx?.openDelete(row) }
		],
		fields,
		load: async () => {
			const result = await couponFeedsAdminApi.list();
			if (result.ok) {
				const data = (result.data as FeedSource[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load feed sources'));
		},
		create: async (body) => {
			const res = await couponFeedsAdminApi.create(toPayload(body));
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await couponFeedsAdminApi.update(id, toPayload(body));
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await couponFeedsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: FeedView
	});
</script>

<svelte:head>
	<title>{t('coupon_feeds.title', 'Deal Feeds')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}
{#if pageNotice}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
	>
		{pageNotice}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#snippet FeedView({ row }: { row: FeedSource })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.source_type', 'Source Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.source_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.endpoint_url', 'Endpoint URL')}</p>
			<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.endpoint_url}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.api_key', 'API Key')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.has_api_key ? t('coupon_feeds.api_key_set', 'Configured') : t('coupon_feeds.api_key_none', 'None')}</p>
		</div>
		{#if row.default_provider_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.default_provider', 'Default Provider')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{getProviderSelectedLabel(row.default_provider_id)}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.last_sync', 'Last Sync')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.last_synced_at ? new Date(row.last_synced_at).toLocaleString() : t('coupon_feeds.never_synced', 'Never')}
			</p>
		</div>
		{#if row.last_error}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.last_error', 'Last Error')}</p>
				<p class="mt-1 break-all text-sm" style="color: var(--color-danger)">{row.last_error}</p>
			</div>
		{/if}
		{#if row.stats}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.stats', 'Last Run Stats')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.stats.seen ?? 0} seen · {row.stats.created ?? 0} created · {row.stats.updated ?? 0} updated · {row.stats.suspended ?? 0} suspended · {row.stats.expired ?? 0} expired · {row.stats.skipped ?? 0} skipped
				</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('coupon_feeds.coupons', 'Coupons')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.coupon_count ?? 0}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.enabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}</p>
		</div>
	</div>
{/snippet}
