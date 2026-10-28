<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /academy/revenue-splits — read-only audit viewer migrated to EntityPage scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { academyAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t, locale } from '$lib/stores/i18n.svelte';
	import { Eye } from 'lucide-svelte';

	type RevenueSplit = {
		id: string;
		instructor_id: string;
		instructor_name?: string;
		track_id: string;
		track_title?: string;
		source: 'purchase' | 'pro_stream';
		gross_kobo: number;
		platform_commission_kobo: number;
		instructor_net_kobo: number;
		payout_id?: string | null;
		created_at: string;
	};

	// Page-local date-range filter (EntityPage FilterDef only supports select dropdowns).
	let dateFrom = $state('');
	let dateTo = $state('');
	let entityController = $state<EntityPageController | undefined>(undefined);

	let revenueSourceOptions = $state<CatalogOption[]>([
		{ value: 'purchase', label: t('academy.source_purchase', 'Purchase') },
		{ value: 'pro_stream', label: t('academy.source_pro_stream', 'Pro Stream') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('academy.revenue_splits.source');
			if (res.ok && res.data?.length) revenueSourceOptions = res.data;
		})();
	});

	function formatKobo(kobo: number): string {
		const major = kobo / 100;
		return `₦ ${major.toLocaleString(locale.current, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
	}

	function applyDateFilter() {
		entityController?.reload();
	}

	const config = $derived<EntityPageConfig>({
		title: t('academy.revenue_splits_title', 'Revenue Splits'),
		description: t('academy.revenue_splits_desc', 'Read-only audit viewer for per-event revenue split records'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'instructor_name',
				label: t('academy.instructor', 'Instructor'),
				sortable: true,
				render: (v: string, row: RevenueSplit) =>
					`<span class="text-sm">${v || (row.instructor_id ? row.instructor_id.slice(0, 8) + '...' : '—')}</span>`
			},
			{
				key: 'track_title',
				label: t('academy.track_title', 'Track'),
				render: (v: string) => `<span class="text-sm">${v ?? '—'}</span>`
			},
			{
				key: 'source',
				label: t('academy.source', 'Source'),
				sortable: true,
				componentSnippet: SourceBadge
			},
			{
				key: 'gross_kobo',
				label: t('academy.gross', 'Gross'),
				sortable: true,
				render: (v: number) => `<span class="text-sm font-medium">${formatKobo(v)}</span>`
			},
			{
				key: 'platform_commission_kobo',
				label: t('academy.commission', 'Commission'),
				render: (v: number) =>
					`<span class="text-xs" style="color: var(--color-muted)">${formatKobo(v)}</span>`
			},
			{
				key: 'instructor_net_kobo',
				label: t('academy.net', 'Instructor Net'),
				sortable: true,
				render: (v: number) =>
					`<span class="text-sm font-semibold" style="color: var(--color-success)">${formatKobo(v)}</span>`
			},
			{
				key: 'payout_id',
				label: t('academy.payout_id', 'Payout ID'),
				render: (v: string | null | undefined) =>
					v
						? `<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 8)}...</span>`
						: `<span class="text-xs" style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'source',
				label: t('academy.source', 'Source'),
				options: revenueSourceOptions
			}
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: RevenueSplit, ctx) => ctx?.openView(row) }
		],
		fields: [],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams();
			if (filter.source) params.set('source', filter.source);
			if (dateFrom) params.set('from', dateFrom);
			if (dateTo) params.set('to', dateTo);
			const result = await academyAdminApi.listRevenueSplits(params.toString() || undefined);
			if (!result.ok) {
				throw new Error(result.error ?? t('academy.revenue_splits_load_error', 'Failed to load revenue splits'));
			}
			const raw = (result.data as { data?: RevenueSplit[] } | null | undefined)?.data ?? [];

			// Client-side search (API has no q param).
			let filtered = raw;
			if (q) {
				const ql = q.toLowerCase();
				filtered = filtered.filter(
					(s) =>
						s.instructor_id.toLowerCase().includes(ql) ||
						(s.track_title ?? '').toLowerCase().includes(ql) ||
						(s.payout_id ?? '').toLowerCase().includes(ql)
				);
			}

			// Client-side sort (API has no sort param).
			if (sort) {
				const [key, dir] = sort.split(':');
				filtered = [...filtered].sort((a, b) => {
					const av = a[key as keyof RevenueSplit] as any;
					const bv = b[key as keyof RevenueSplit] as any;
					if (av < bv) return dir === 'asc' ? -1 : 1;
					if (av > bv) return dir === 'asc' ? 1 : -1;
					return 0;
				});
			}

			const total = filtered.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			const paged = filtered.slice(start, start + pageSize);
			return { data: paged, total, totalPages };
		},
		viewSnippet: SplitView
	});
</script>

<svelte:head>
	<title>{t('academy.revenue_splits_title', 'Revenue Splits')} — {siteName()} Admin</title>
</svelte:head>

<!-- Page-local date-range filter (EntityPage FilterDef only supports select dropdowns) -->
<div class="mb-4 flex flex-wrap items-center gap-3">
	<div class="flex items-center gap-2">
		<label for="date-from" class="text-sm" style="color: var(--color-muted)">
			{t('common.from', 'From')}
		</label>
		<input
			id="date-from"
			type="date"
			bind:value={dateFrom}
			onchange={applyDateFilter}
			class="h-9 rounded-lg border px-3 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
		/>
	</div>
	<div class="flex items-center gap-2">
		<label for="date-to" class="text-sm" style="color: var(--color-muted)">
			{t('common.to', 'To')}
		</label>
		<input
			id="date-to"
			type="date"
			bind:value={dateTo}
			onchange={applyDateFilter}
			class="h-9 rounded-lg border px-3 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
		/>
	</div>
</div>

<EntityPage {config} bind:controller={entityController} />

{#snippet SourceBadge({ value }: { value: string; row: RevenueSplit })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium
		{value === 'pro_stream' ? 'bg-info/15 text-info' : 'bg-primary/15 text-primary'}"
	>
		{value}
	</span>
{/snippet}

{#snippet SplitView({ row }: { row: RevenueSplit })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.instructor', 'Instructor')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.instructor_name || row.instructor_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.track_title', 'Track')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.track_title ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.source', 'Source')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.source}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.gross', 'Gross')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatKobo(row.gross_kobo)}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.commission', 'Commission')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatKobo(row.platform_commission_kobo)}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.net', 'Instructor Net')}</p>
			<p class="mt-1 text-sm font-semibold" style="color: var(--color-success)">{formatKobo(row.instructor_net_kobo)}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('academy.payout_id', 'Payout ID')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.payout_id ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleDateString()}</p>
		</div>
	</div>
{/snippet}
