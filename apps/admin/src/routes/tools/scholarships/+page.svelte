<script lang="ts">
	// /tools/scholarships — monitoring page for scholarship finder: stats + deadlines list + reminder log summary.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { scholarshipAdminApi } from '$lib/api';

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
			// Non-fatal: the deadlines list still loads via EntityPage.
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: DeadlineItem, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.tools.col_title', 'Title'), type: 'text' },
		{ key: 'deadline', label: t('app.tools.col_deadline', 'Deadline'), type: 'date' },
		{ key: 'status', label: t('app.tools.col_status', 'Status'), type: 'text' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.scholarships_heading', 'Scholarship Deadlines'),
		description: t('app.tools.scholarships_desc', 'Monitor scholarship deadlines and reminder activity'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.tools.col_title', 'Title'), sortable: true },
			{
				key: 'deadline',
				label: t('app.tools.col_deadline', 'Deadline'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'days_until',
				label: t('app.tools.col_days_until', 'Days Until'),
				sortable: true,
				render: (value: number) => (value != null ? String(value) : '—')
			},
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await scholarshipAdminApi.deadlines('page_size=100');
			if (result.ok) {
				const data = (result.data as DeadlineItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: DeadlineView
	});
</script>

<svelte:head>
	<title>{t('app.tools.scholarships_page_title', 'Scholarship Finder — JapaConnect Admin')}</title>
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
{#snippet DeadlineView({ row }: { row: DeadlineItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_deadline', 'Deadline')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.deadline ? new Date(row.deadline).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_days_until', 'Days Until')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.days_until != null ? String(row.days_until) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
	</div>
{/snippet}
