<script lang="ts">
	// /tools/scholarships — full CRUD admin page for scholarship projections.
	// Combines the summary dashboard (stats + deadlines + reminder log) with
	// an EntityPage CRUD list backed by /admin/scholarships.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2, Eye, Pencil } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { scholarshipAdminApi } from '$lib/api';

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
		days_until?: number;
		status: string;
	};

	const canManage = $derived(hasPermission('scholarship.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);
	let stats = $state<Record<string, any>>({});
	let reminderLogEntries = $state<any[]>([]);

	// Load stats and reminder log on mount.
	$effect(() => {
		void loadSummary();
	});

	async function loadSummary() {
		pageError = '';
		try {
			const [statsRes, logRes] = await Promise.all([
				scholarshipAdminApi.stats(),
				scholarshipAdminApi.reminderLog('page_size=100')
			]);
			if (statsRes.ok) stats = (statsRes.data as Record<string, any>) ?? {};
			if (logRes.ok) reminderLogEntries = (logRes.data as any[]) ?? [];
		} catch {
			// Non-fatal: the CRUD list still loads via EntityPage.
		}
	}

	// Stat card entries derived from the stats object.
	const statCards = $derived.by<Array<{ key: string; label: string; value: string }>>(() => {
		const entries: Array<{ key: string; label: string; value: string }> = [];
		const labels: Record<string, string> = {
			total_scholarships: t('app.tools.col_total_scholarships', 'Total Scholarships'),
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
	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.tools.col_title', 'Title'), type: 'text', required: true },
		{ key: 'country', label: t('app.tools.col_country', 'Country'), type: 'text', required: true, placeholder: 'Germany' },
		{ key: 'university', label: t('app.tools.col_university', 'University'), type: 'text' },
		{ key: 'degree_level', label: t('app.tools.col_degree_level', 'Degree Level'), type: 'text', placeholder: 'masters, bachelors, phd' },
		{ key: 'deadline', label: t('app.tools.col_deadline', 'Deadline'), type: 'date' },
		{ key: 'application_url', label: t('app.tools.col_application_url', 'Application URL'), type: 'url' },
		{ key: 'source_url', label: t('app.tools.col_source_url', 'Source URL'), type: 'url', required: true },
		{ key: 'source_type', label: t('app.tools.col_source_type', 'Source Type'), type: 'text', placeholder: 'web, rss, api' },
		{ key: 'trust_level', label: t('app.tools.col_trust_level', 'Trust Level'), type: 'number' },
		{ key: 'description', label: t('app.tools.col_description', 'Description'), type: 'textarea' }
	];

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
				sortable: true,
				render: (v: string) => `<span class="font-medium text-foreground">${v ?? '—'}</span>`
			},
			{ key: 'country', label: t('app.tools.col_country', 'Country'), sortable: true },
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
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'trust_level',
				label: t('app.tools.col_trust_level', 'Trust'),
				sortable: true,
				render: (v: number) => (v != null ? String(v) : '—')
			},
			{
				key: 'verification_status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
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
				sortable: true,
				render: (v: number) => (v != null ? `${v}%` : '—')
			}
		],
		filters: [
			{
				key: 'verification_status',
				label: t('app.tools.col_status', 'Status'),
				options: [
					{ value: 'verified', label: 'Verified' },
					{ value: 'needs_review', label: 'Needs Review' },
					{ value: 'conflict', label: 'Conflict' },
					{ value: 'stale', label: 'Stale' }
				]
			}
		],
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
