<script lang="ts">
	// /(app)/jobs — migrated to the EntityPage scaffold; read-only list with a feature-toggle state action.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { api, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Star, Eye } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type JobApplication = {
		id: string;
		job_id: string;
		job_title: string;
		job_slug: string;
		applicant_id: string;
		applicant_username: string;
		applicant_email: string;
		status: string;
		is_featured?: boolean;
		employer_notes: string | null;
		applied_at: string;
		status_updated_at: string;
	};

	type StatusDist = Record<string, number>;

	const canManage = $derived(hasPermission('jobs.manage'));

	// Status distribution cards are a dashboard element rendered above EntityPage; captured from the load response as a side effect.
	let statusDist = $state<StatusDist>({});

	// ── Feature-toggle state action ───── EntityPage's confirm Modal only covers default delete; state actions keep page-local Modals, confirm calls API + controller.reload().
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let togglingId = $state('');
	let showFeatureConfirm = $state(false);
	let featureTarget = $state<JobApplication | null>(null);

	function promptFeature(app: JobApplication) {
		featureTarget = app;
		showFeatureConfirm = true;
	}

	async function confirmToggleFeature() {
		if (!featureTarget) return;
		if (togglingId) return; // in-flight guard
		togglingId = featureTarget.id;
		pageError = '';
		try {
			const result = await api(`/admin/jobs/${featureTarget.id}/feature`, { method: 'PATCH' });
			if (result.ok) {
				showFeatureConfirm = false;
				featureTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('jobs.toggle_error', 'Failed to toggle feature');
		} finally {
			togglingId = '';
		}
	}

	// ── Row actions ───── Custom actions replace defaults, so View is included explicitly; feature toggle is permission-gated.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: JobApplication, ctx) => ctx?.openView(row) }
		];
		if (canManage) {
			acts.push({
				label: t('jobs.toggle_feature', 'Toggle Featured'),
				icon: Star,
				onclick: (row: JobApplication) => promptFeature(row),
				show: () => true
			});
		}
		return acts;
	});

	// ── Filters ─────
	const filters: FilterDef[] = [
		{
			key: 'status',
			label: t('jobs.status', 'Status'),
			options: [
				{ value: 'applied', label: t('jobs.status_applied', 'Applied') },
				{ value: 'reviewing', label: t('jobs.status_reviewing', 'Reviewing') },
				{ value: 'shortlisted', label: t('jobs.status_shortlisted', 'Shortlisted') },
				{ value: 'rejected', label: t('jobs.status_rejected', 'Rejected') },
				{ value: 'offered', label: t('jobs.status_offered', 'Offered') },
				{ value: 'withdrawn', label: t('jobs.status_withdrawn', 'Withdrawn') }
			]
		}
	];

	const config = $derived<EntityPageConfig>({
		title: t('jobs.title', 'Jobs Board'),
		description: t('jobs.description', 'Manage job applications and featured listings.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'job_title',
				label: t('jobs.title', 'Job Title'),
				sortable: true,
				render: (v: string) => `<span class="font-medium text-foreground">${v ?? '—'}</span>`
			},
			{
				key: 'applicant_username',
				label: t('jobs.applicant', 'Applicant'),
				sortable: true,
				render: (v: string) => `<span class="text-foreground">${v ?? '—'}</span>`
			},
			{
				key: 'applicant_email',
				label: t('jobs.email', 'Email'),
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ?? '—'}</span>`
			},
			{
				key: 'status',
				label: t('jobs.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const clsMap: Record<string, string> = {
						applied: 'bg-primary/15 text-primary border-primary',
						reviewing: 'bg-warning/15 text-warning border-warning',
						shortlisted: 'bg-success/15 text-success border-success',
						rejected: 'bg-danger/15 text-danger border-danger',
						offered: 'bg-success/15 text-success border-success',
						withdrawn: 'bg-muted text-muted-foreground border-border'
					};
					const cls = clsMap[v] ?? 'bg-muted text-muted-foreground border-border';
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'applied_at',
				label: t('jobs.applied', 'Applied'),
				sortable: true,
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters,
		actions: rowActions,
		fields: [],
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (page) params.set('page', String(page));
			if (pageSize) params.set('limit', String(pageSize));
			const result = await apiRaw<{ data: JobApplication[]; meta?: { total?: number }; status_distribution?: StatusDist }>(
				`/admin/jobs${params.toString() ? `?${params.toString()}` : ''}`
			);
			if (result.ok) {
				const body = result.data;
				const data = body?.data ?? [];
				statusDist = body?.status_distribution ?? {};
				const total = body?.meta?.total ?? data.length;
				const totalPages = total > 0 && pageSize ? Math.ceil(total / pageSize) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('jobs.load_error', 'Failed to load job applications'));
		},
		viewSnippet: JobView
	});
</script>

<svelte:head>
	<title>{t('jobs.title', 'Jobs Board')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

{#if Object.keys(statusDist).length > 0}
	<div class="mb-6 grid gap-3 sm:grid-cols-4">
		{#each Object.entries(statusDist) as [status, count]}
			<Card>
				<CardHeader>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{status}</span>
				</CardHeader>
				<CardContent>
					<p class="text-2xl font-bold" style="color: var(--color-foreground)">{count}</p>
				</CardContent>
			</Card>
		{/each}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Feature toggle confirmation (state action behind page-local Modal) -->
<Modal bind:open={showFeatureConfirm} title={t('jobs.toggle_feature', 'Toggle Featured Status')} size="sm">
	{#if featureTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('jobs.toggle_feature_confirm', 'Toggle featured status for')}
			<strong>{featureTarget.job_title}</strong>?
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showFeatureConfirm = false)} disabled={togglingId !== ''}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmToggleFeature} disabled={togglingId !== ''}>
			<Star class="mr-2 h-4 w-4" />
			{togglingId !== '' ? t('common.processing', 'Processing...') : t('common.confirm', 'Confirm')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet JobView({ row }: { row: JobApplication })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.title', 'Job Title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.job_title ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.applicant', 'Applicant')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.applicant_username ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.email', 'Email')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.applicant_email ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.featured', 'Featured')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_featured ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.applied', 'Applied')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.applied_at ? new Date(row.applied_at).toLocaleString() : '—'}</p>
			</div>
		</div>
		{#if row.employer_notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.employer_notes', 'Employer Notes')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.employer_notes}</p>
			</div>
		{/if}
	</div>
{/snippet}
