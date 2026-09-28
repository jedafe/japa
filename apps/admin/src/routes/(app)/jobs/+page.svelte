<script lang="ts">
	// /(app)/jobs — admin view of all job listings (Section 63, ADR-081).
	// Full CRUD: list, edit, delete, feature-toggle.
	// Ref: japaconnect-engineering/sections/63-jobs-board.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { jobsAdminApi, contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Star, Eye, Pencil, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type JobListing = {
		id: string;
		title: string;
		slug: string;
		summary: string;
		status: string;
		is_featured: boolean;
		organization_id?: string;
		author_id?: string;
		field_values: Record<string, string>;
		published_at: string;
		created_at: string;
	};

	type StatusDist = Record<string, number>;

	const canManage = $derived(hasPermission('jobs.manage'));

	// Status distribution cards are a dashboard element rendered above EntityPage;
	// captured from the load response as a side effect.
	let statusDist = $state<StatusDist>({});

	// Job listings are content rows; load canonical content status options (Rule 16).
	let statusOptions = $state<{ value: string; label: string }[]>([
		{ value: 'published', label: t('jobs.status_published', 'Published') },
		{ value: 'archived', label: t('jobs.status_archived', 'Archived') },
		{ value: 'draft', label: t('jobs.status_draft', 'Draft') }
	]);

	$effect(() => {
		void (async () => {
			const res = await contentAdminApi.getStatusOptions();
			if (res.ok && res.data?.length) {
				statusOptions = res.data;
			}
		})();
	});

	// ── Feature-toggle state action ─────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let togglingId = $state('');
	let showFeatureConfirm = $state(false);
	let featureTarget = $state<JobListing | null>(null);
	let bulkDeleting = $state(false);

	function promptFeature(listing: JobListing) {
		featureTarget = listing;
		showFeatureConfirm = true;
	}

	async function confirmToggleFeature() {
		if (!featureTarget) return;
		if (togglingId) return; // in-flight guard
		togglingId = featureTarget.id;
		pageError = '';
		try {
			const result = await jobsAdminApi.toggleFeature(featureTarget.id);
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

	// ── Bulk delete ─────
	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => jobsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete jobs');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Row actions ─────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: JobListing, ctx) => ctx?.openView(row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: JobListing, ctx) => ctx?.openEdit(row)
			});
			acts.push({
				label: t('jobs.toggle_feature', 'Toggle Featured'),
				icon: Star,
				onclick: (row: JobListing) => promptFeature(row)
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: JobListing, ctx) => ctx?.openDelete(row)
			});
		}
		return acts;
	});

	// ── Filters ───── Listing status filter (published, archived, draft).
	const filters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('jobs.status', 'Status'),
			options: statusOptions
		}
	]);

	// ── Editable fields ─────
	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('jobs.title_col', 'Title'), type: 'text', required: true },
		{ key: 'status', label: t('jobs.status', 'Status'), type: 'select', options: statusOptions },
		{ key: 'body', label: t('jobs.body', 'Body'), type: 'textarea' }
	]);

	// Helper to read a custom field value from the listing's field_values map.
	function field(row: JobListing, key: string): string {
		return row.field_values?.[key] ?? '';
	}

	// Helper to build a location string from city + country.
	function locationStr(row: JobListing): string {
		return [field(row, 'city'), field(row, 'country')].filter(Boolean).join(', ');
	}

	const config = $derived<EntityPageConfig>({
		title: t('jobs.title', 'Jobs Board'),
		description: t('jobs.description', 'Manage job listings and featured placements.'),
		createLabel: t('common.create', 'Create'),
		createPermission: 'jobs.manage',
		editPermission: 'jobs.manage',
		deletePermission: 'jobs.manage',
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('jobs.title_col', 'Title'),
				sortable: true,
				render: (v: string) => `<span class="font-medium text-foreground">${v ?? '—'}</span>`
			},
			{
				key: 'field_company',
				label: t('jobs.company', 'Company'),
				render: (_v: string, row: JobListing) => {
					const company = field(row, 'company_name');
					return `<span class="text-foreground">${company || '—'}</span>`;
				}
			},
			{
				key: 'field_type',
				label: t('jobs.job_type', 'Type'),
				render: (_v: string, row: JobListing) => {
					const jobType = field(row, 'job_type');
					return `<span class="text-muted-foreground text-xs">${jobType || '—'}</span>`;
				}
			},
			{
				key: 'field_location',
				label: t('jobs.location', 'Location'),
				render: (_v: string, row: JobListing) => {
					const loc = locationStr(row);
					return `<span class="text-muted-foreground text-xs">${loc || '—'}</span>`;
				}
			},
			{
				key: 'status',
				label: t('jobs.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const clsMap: Record<string, string> = {
						published: 'bg-success/15 text-success border-success',
						archived: 'bg-muted text-muted-foreground border-border',
						draft: 'bg-warning/15 text-warning border-warning'
					};
					const cls = clsMap[v] ?? 'bg-muted text-muted-foreground border-border';
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'is_featured',
				label: t('jobs.featured', 'Featured'),
				render: (v: boolean) =>
					v
						? `<span class="inline-flex items-center gap-1 text-xs text-warning"><svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg> ${t('common.yes', 'Yes')}</span>`
						: `<span class="text-xs text-muted-foreground">${t('common.no', 'No')}</span>`
			},
			{
				key: 'created_at',
				label: t('jobs.created', 'Created'),
				sortable: true,
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters,
		actions: rowActions,
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
			if (filter.status) params.set('status', filter.status);
			if (page) params.set('page', String(page));
			if (pageSize) params.set('limit', String(pageSize));
			const result = await jobsAdminApi.list(params.toString());
			if (result.ok) {
				const body = result.data;
				const data = (body?.data as JobListing[]) ?? [];
				statusDist = body?.status_distribution ?? {};
				const total = body?.meta?.total ?? data.length;
				const totalPages = total > 0 && pageSize ? Math.ceil(total / pageSize) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('jobs.load_error', 'Failed to load job listings'));
		},
		update: async (id, body) => {
			const result = await jobsAdminApi.update(id, body);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		remove: async (id) => {
			const result = await jobsAdminApi.delete(id);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
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
		{#each Object.entries(statusDist) as [status, count] (status)}
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
			<strong>{featureTarget.title}</strong>?
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
{#snippet JobView({ row }: { row: JobListing })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.title_col', 'Title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.company', 'Company')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{field(row, 'company_name') || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.job_type', 'Job Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{field(row, 'job_type') || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.location', 'Location')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{locationStr(row) || '—'}</p>
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
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.salary', 'Salary Range')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{[field(row, 'salary_min'), field(row, 'salary_max')].filter(Boolean).join('–') || '—'}
					{#if field(row, 'salary_currency')}<span class="text-muted-foreground text-xs"> {field(row, 'salary_currency')}</span>{/if}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.created', 'Created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
			</div>
		</div>
		{#if row.summary}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.summary', 'Summary')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary}</p>
			</div>
		{/if}
		{#if field(row, 'skills_required')}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.skills', 'Skills Required')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{field(row, 'skills_required')}</p>
			</div>
		{/if}
		{#if field(row, 'visa_sponsorship') === 'true'}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.visa_sponsorship', 'Visa Sponsorship')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{t('common.yes', 'Yes')}</p>
			</div>
		{/if}
	</div>
{/snippet}
