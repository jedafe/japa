<script lang="ts">
	// /tools/scholarships — full CRUD admin page for scholarship projections.
	// Combines the summary dashboard (stats + deadlines + reminder log) with
	// an EntityPage CRUD list backed by /admin/scholarships.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2, Eye, Pencil } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { scholarshipAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';

	type Scholarship = {
		id: string;
		title: string;
		country: string;
		university?: string | null;
		degree_level?: string | null;
		deadline?: string | null;
		source_url?: string | null;
		trust_level: number;
		verification_status: string;
		freshness_score: number;
		created_at?: string;
	};

	type DeadlineItem = {
		id: string;
		title: string;
		deadline?: string;
		days_until?: number | null;
		status: string;
		username?: string | null;
	};

	type ReminderLogEntry = {
		id: string;
		guide_title?: string;
		username?: string | null;
		status: string;
		last_reminded_at?: string | null;
	};

	type StatusDistEntry = { status: string; count: number };

	type TopSavedItem = { guide_id: string; guide_title?: string; save_count: number };

	const canManage = $derived(hasPermission('scholarship.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	const countryUtilsName = useCountries('name');
	$effect(() => {
		countryUtils.load();
	});

	let scholarshipVerificationOptions = $state<CatalogOption[]>([
		{ value: 'verified', label: 'Verified' },
		{ value: 'needs_review', label: 'Needs Review' },
		{ value: 'conflict', label: 'Conflict' },
		{ value: 'stale', label: 'Stale' }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('scholarships.verification_status');
			if (res.ok && res.data?.length) scholarshipVerificationOptions = res.data;
		})();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);
	let stats = $state<Record<string, any>>({});
	let distribution = $state<StatusDistEntry[]>([]);
	let reminderLogEntries = $state<ReminderLogEntry[]>([]);
	let deadlineEntries = $state<DeadlineItem[]>([]);
	let topSavedEntries = $state<TopSavedItem[]>([]);

	// Load stats, deadlines, reminder log, and top-saved on mount.
	$effect(() => {
		void loadSummary();
	});

	async function loadSummary() {
		pageError = '';
		try {
			const [statsRes, logRes, deadlinesRes, topSavedRes] = await Promise.all([
				scholarshipAdminApi.stats(),
				scholarshipAdminApi.reminderLog('page_size=10'),
				scholarshipAdminApi.deadlines('page_size=10'),
				scholarshipAdminApi.topSaved('page_size=10')
			]);
			if (statsRes.ok) {
				const s = (statsRes.data as Record<string, any>) ?? {};
				// Distribution is an array, not a scalar — pull it out so it is
				// rendered as a breakdown instead of a useless "—" stat card.
				distribution = Array.isArray(s.distribution) ? (s.distribution as StatusDistEntry[]) : [];
				delete s.distribution;
				stats = s;
			}
			if (logRes.ok) reminderLogEntries = (logRes.data as ReminderLogEntry[]) ?? [];
			if (deadlinesRes.ok) deadlineEntries = (deadlinesRes.data as DeadlineItem[]) ?? [];
			if (topSavedRes.ok) topSavedEntries = (topSavedRes.data as TopSavedItem[]) ?? [];
		} catch {
			// Non-fatal: the CRUD list still loads via EntityPage.
		}
	}

	// Stat card entries derived from the stats object (scalar values only —
	// distribution is rendered separately as a per-status breakdown).
	const statCards = $derived.by<Array<{ key: string; label: string; value: string }>>(() => {
		const entries: Array<{ key: string; label: string; value: string }> = [];
		const labels: Record<string, string> = {
			// Counts saved scholarships (user saves), not the catalog size.
			total_scholarships: t('app.tools.col_total_saves', 'Total Saves'),
			upcoming_deadlines: t('app.tools.col_upcoming_deadlines', 'Upcoming Deadlines'),
			reminders_sent: t('app.tools.col_reminders_sent', 'Reminders Sent'),
			unique_users: t('app.tools.col_unique_users', 'Unique Users')
		};
		for (const [key, value] of Object.entries(stats)) {
			entries.push({
				key,
				label: labels[key] ?? key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
				value: value == null ? '—' : typeof value === 'number' ? value.toLocaleString() : String(value)
			});
		}
		return entries;
	});

	// Per-status badge classes for the distribution breakdown.
	const statusBadgeClass = (status: string): string => {
		const clsMap: Record<string, string> = {
			saved: 'bg-muted text-muted-foreground border-border',
			researching: 'bg-info/15 text-info border-info',
			applied: 'bg-warning/15 text-warning border-warning',
			accepted: 'bg-success/15 text-success border-success',
			rejected: 'bg-danger/15 text-danger border-danger',
			withdrawn: 'bg-muted text-muted-foreground border-border'
		};
		return clsMap[status] ?? 'bg-muted text-muted-foreground border-border';
	};

	const formatDateTime = (v?: string | null): string =>
		v ? new Date(v).toLocaleString() : '—';

	// ── Bulk delete ─────
	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => scholarshipAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete scholarships');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Fields ───── drives EntityForm rendering + edit-form seeding.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtilsName.ready;
		return [
			{ key: 'title', label: t('app.tools.col_title', 'Title'), type: 'text', required: true },
			{ key: 'country', label: t('app.tools.col_country', 'Country'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtilsName.fetcher, initialFetcher: countryUtilsName.initialFetcher, getSelectedLabel: countryUtilsName.getSelectedLabel, required: true },
			{ key: 'university', label: t('app.tools.col_university', 'University'), type: 'text' },
			{ key: 'degree_level', label: t('app.tools.col_degree_level', 'Degree Level'), type: 'text', placeholder: 'masters, bachelors, phd' },
			{ key: 'deadline', label: t('app.tools.col_deadline', 'Deadline'), type: 'date' },
			{ key: 'application_url', label: t('app.tools.col_application_url', 'Application URL'), type: 'url' },
			{ key: 'source_url', label: t('app.tools.col_source_url', 'Source URL'), type: 'url', required: true },
			{ key: 'source_type', label: t('app.tools.col_source_type', 'Source Type'), type: 'text', placeholder: 'web, rss, api' },
			{ key: 'trust_level', label: t('app.tools.col_trust_level', 'Trust Level'), type: 'number' },
			{ key: 'description', label: t('app.tools.col_description', 'Description'), type: 'textarea' }
		];
	});

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'verification_status',
				label: t('app.tools.col_status', 'Status'),
				options: scholarshipVerificationOptions
			},
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.scholarships_heading', 'Scholarships'),
		description: t('app.tools.scholarships_desc', 'Manage scholarship projections — create, edit, delete, and monitor deadlines.'),
		createLabel: t('common.create', 'Create'),
		createPermission: 'scholarship.manage',
		editPermission: 'scholarship.manage',
		deletePermission: 'scholarship.manage',
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('app.tools.col_title', 'Title'),
				render: (v: string) => `<span class="font-medium text-foreground">${v ?? '—'}</span>`
			},
			{ key: 'country', label: t('app.tools.col_country', 'Country') },
			{
				key: 'university',
				label: t('app.tools.col_university', 'University'),
				render: (v: string) => v || '—'
			},
			{
				key: 'degree_level',
				label: t('app.tools.col_degree_level', 'Degree Level'),
				render: (v: string) => v || '—'
			},
			{
				key: 'deadline',
				label: t('app.tools.col_deadline', 'Deadline'),
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'trust_level',
				label: t('app.tools.col_trust_level', 'Trust'),
				render: (v: number) => (v != null ? String(v) : '—')
			},
			{
				key: 'verification_status',
				label: t('app.tools.col_status', 'Status'),
				render: (v: string) => {
					const clsMap: Record<string, string> = {
						verified: 'bg-success/15 text-success border-success',
						needs_review: 'bg-warning/15 text-warning border-warning',
						conflict: 'bg-danger/15 text-danger border-danger',
						stale: 'bg-muted text-muted-foreground border-border'
					};
					const cls = clsMap[v] ?? 'bg-muted text-muted-foreground border-border';
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'freshness_score',
				label: t('app.tools.col_freshness', 'Freshness'),
				render: (v: number) => (v != null ? `${v}%` : '—')
			}
		],
		filters,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.verification_status) params.set('verification_status', filter.verification_status);
			if (filter.dest) params.set('dest', filter.dest);
			if (page) params.set('page', String(page));
			if (pageSize) params.set('page_size', String(pageSize));
			const result = await scholarshipAdminApi.list(params.toString());
			if (result.ok) {
				const body = result.data;
				const data = (body?.data as Scholarship[]) ?? [];
				const total = body?.meta?.count ?? data.length;
				const totalPages = total > 0 && pageSize ? Math.ceil(total / pageSize) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load scholarships'));
		},
		create: async (body) => {
			const result = await scholarshipAdminApi.create(body);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		update: async (id, body) => {
			const result = await scholarshipAdminApi.update(id, body);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		remove: async (id) => {
			const result = await scholarshipAdminApi.delete(id);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		viewSnippet: ScholarshipView
	});
</script>

<svelte:head>
	<title>{t('app.tools.scholarships_page_title', 'Scholarships — JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<!-- ── Summary stat cards ────────────────────────────────────────────────── -->
{#if statCards.length > 0}
	<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		{#each statCards as card (card.key)}
			<div
				class="rounded-lg border p-5"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{card.label}
				</p>
				<p class="mt-2 text-2xl font-bold" style="color: var(--color-foreground)">
					{card.value}
				</p>
			</div>
		{/each}
	</div>
{/if}

<!-- ── Distribution breakdown + deadlines + reminder log + top saved ─────── -->
<div class="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-4">
	<!-- Per-status distribution of saved scholarships -->
	<div
		class="rounded-lg border p-5"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
			{t('app.tools.col_status_distribution', 'Status Distribution')}
		</p>
		{#if distribution.length > 0}
			<ul class="mt-3 space-y-2">
				{#each distribution as d (d.status)}
					<li class="flex items-center justify-between">
						<span
							class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium {statusBadgeClass(d.status)}"
						>
							{d.status}
						</span>
						<span class="text-sm font-semibold" style="color: var(--color-foreground)">
							{d.count.toLocaleString()}
						</span>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('app.tools.no_saved_scholarships', 'No saved scholarships yet.')}
			</p>
		{/if}
	</div>

	<!-- Upcoming deadlines -->
	<div
		class="rounded-lg border p-5"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
			{t('app.tools.col_upcoming_deadlines', 'Upcoming Deadlines')}
		</p>
		{#if deadlineEntries.length > 0}
			<ul class="mt-3 space-y-2">
				{#each deadlineEntries as d (d.id)}
					<li>
						<div class="flex items-center justify-between gap-2">
							<span class="truncate text-sm font-medium" style="color: var(--color-foreground)">
								{d.title || '—'}
							</span>
							{#if d.days_until != null}
								<span
									class="shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-medium {d.days_until <= 7
										? 'bg-danger/15 text-danger border-danger'
										: d.days_until <= 30
											? 'bg-warning/15 text-warning border-warning'
											: 'bg-muted text-muted-foreground border-border'}"
								>
									{d.days_until}d
								</span>
							{/if}
						</div>
						<p class="text-xs" style="color: var(--color-muted)">
							{d.deadline ? new Date(d.deadline).toLocaleDateString() : '—'}{#if d.username} · {d.username}{/if}
						</p>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('app.tools.no_upcoming_deadlines', 'No upcoming deadlines.')}
			</p>
		{/if}
	</div>

	<!-- Reminder delivery log -->
	<div
		class="rounded-lg border p-5"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
			{t('app.tools.col_reminder_log', 'Reminder Log')}
		</p>
		{#if reminderLogEntries.length > 0}
			<ul class="mt-3 space-y-2">
				{#each reminderLogEntries as r (r.id)}
					<li>
						<div class="flex items-center justify-between gap-2">
							<span class="truncate text-sm font-medium" style="color: var(--color-foreground)">
								{r.guide_title || '—'}
							</span>
							<span
								class="shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-medium {statusBadgeClass(r.status)}"
							>
								{r.status}
							</span>
						</div>
						<p class="text-xs" style="color: var(--color-muted)">
							{formatDateTime(r.last_reminded_at)}{#if r.username} · {r.username}{/if}
						</p>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('app.tools.no_reminders_sent', 'No reminders sent yet.')}
			</p>
		{/if}
	</div>

	<!-- Most-saved scholarships (GET /admin/scholarships/top-saved) -->
	<div
		class="rounded-lg border p-5"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
			{t('app.tools.col_top_saved', 'Top Saved')}
		</p>
		{#if topSavedEntries.length > 0}
			<ul class="mt-3 space-y-2">
				{#each topSavedEntries as s (s.guide_id)}
					<li class="flex items-center justify-between gap-2">
						<span class="truncate text-sm font-medium" style="color: var(--color-foreground)">
							{s.guide_title || '—'}
						</span>
						<span
							class="shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-medium bg-muted text-muted-foreground border-border"
						>
							{s.save_count.toLocaleString()}
						</span>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('app.tools.no_saved_scholarships', 'No saved scholarships yet.')}
			</p>
		{/if}
	</div>
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ScholarshipView({ row }: { row: Scholarship })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_title', 'Title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_country', 'Country')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.country || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_university', 'University')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.university || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_degree_level', 'Degree Level')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.degree_level || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_deadline', 'Deadline')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.deadline ? new Date(row.deadline).toLocaleDateString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_trust_level', 'Trust Level')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.trust_level ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.verification_status || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_freshness', 'Freshness')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.freshness_score != null ? `${row.freshness_score}%` : '—'}</p>
			</div>
		</div>
		{#if row.source_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_source_url', 'Source URL')}</p>
				<a href={row.source_url} target="_blank" rel="noopener" class="mt-1 text-sm text-primary underline">{row.source_url}</a>
			</div>
		{/if}
	</div>
{/snippet}
