<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /(app)/jobs — admin view of all job listings (Section 63, ADR-081).
	// Listings are org-created via POST /org/:org_id/jobs — there is no
	// POST /admin/jobs in the route catalog, so this page moderates only:
	// list, view, edit, feature-toggle, delete (hideCreate).
	// Ref: japaconnect-engineering/sections/63-jobs-board.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { jobsAdminApi, contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { loadCurrencies, currencySymbol, currencyOptions, type CurrencyInfo } from '$lib/utils/currency';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Star, Eye, Pencil, Trash2, Users } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type JobListing = {
		id: string;
		title: string;
		slug: string;
		summary: string;
		body: string;
		status: string;
		is_featured: boolean;
		organization_id?: string;
		author_id?: string;
		field_values: Record<string, string>;
		published_at: string;
		created_at: string;
	};

	type FieldOptions = Record<string, { value: string; label: string }[]>;
	type StatusDist = Record<string, number>;

	const canManage = $derived(hasPermission('jobs.manage'));

	// Boolean-typed job_listing_fields keys — custom_field_values stores them as
	// 'true'/'false' strings; the edit form's checkbox fields need real booleans.
	const BOOLEAN_FIELDS = new Set(['visa_sponsorship', 'relocation_assistance', 'is_featured']);

	// Status distribution cards are a dashboard element rendered above EntityPage;
	// captured from the load response as a side effect.
	let statusDist = $state<StatusDist>({});

	// Job listings are content rows; load canonical content status options (Rule 16).
	let statusOptions = $state<{ value: string; label: string }[]>([
		{ value: 'published', label: t('jobs.status_published', 'Published') },
		{ value: 'archived', label: t('jobs.status_archived', 'Archived') },
		{ value: 'draft', label: t('jobs.status_draft', 'Draft') }
	]);

	// §58 — managed currency list for the salary-range symbol and currency select.
	let currencies = $state<CurrencyInfo[]>([]);

	// Canonical select options for job_listing_fields (job_type, job_function,
	// location_type, salary_period, experience_level) — seeded server-side and
	// served by GET /jobs/field-options (Rule 16 — never hardcode option lists).
	let fieldOptions = $state<FieldOptions>({});

	$effect(() => {
		void (async () => {
			const res = await contentAdminApi.getStatusOptions();
			if (res.ok && res.data?.length) {
				statusOptions = res.data;
			}
		})();
		void (async () => {
			const res = await jobsAdminApi.fieldOptions();
			if (res.ok && res.data) {
				fieldOptions = res.data;
			}
		})();
		loadCurrencies().then((list) => (currencies = list));
	});

	function optionsFor(key: string): { value: string; label: string }[] {
		return fieldOptions[key] ?? [];
	}

	function optionLabel(key: string, value: string): string {
		return fieldOptions[key]?.find((o) => o.value === value)?.label ?? value;
	}

	// §14 content statuses rendered through i18n keys (jobs.status_*) so the
	// badge, filter, and edit select follow the active locale; the API label
	// stays as the fallback for statuses without a translation key.
	const localizedStatusOptions = $derived(
		statusOptions.map((o) => ({ value: o.value, label: t(`jobs.status_${o.value}`, o.label) }))
	);

	function statusLabel(status: string): string {
		return t(`jobs.status_${status}`, statusOptions.find((o) => o.value === status)?.label ?? status);
	}

	// ── Feature-toggle state action ─────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let togglingId = $state('');
	let showFeatureConfirm = $state(false);
	let featureTarget = $state<JobListing | null>(null);

	function promptFeature(listing: JobListing) {
		featureTarget = listing;
		showFeatureConfirm = true;
	}

	// ── Applications sub-flow ─────
	// §63 — jobs.manage covers applications: the ⋮ menu exposes a read-only
	// per-listing applications modal backed by
	// GET /admin/jobs/applications?job_id=…
	type JobApplication = {
		id: string;
		applicant_username: string;
		status: string;
		employer_notes?: string;
		applied_at: string;
	};

	let showApplications = $state(false);
	let applicationsTarget = $state<JobListing | null>(null);
	let applications = $state<JobApplication[]>([]);
	let applicationsLoading = $state(false);
	let applicationsError = $state('');

	async function openApplications(listing: JobListing) {
		applicationsTarget = listing;
		applications = [];
		applicationsError = '';
		showApplications = true;
		applicationsLoading = true;
		try {
			const params = new URLSearchParams({ job_id: listing.id, limit: '50' });
			const result = await jobsAdminApi.listApplications(params.toString());
			if (result.ok) {
				applications = (result.data?.data as JobApplication[]) ?? [];
			} else {
				applicationsError = result.error ?? t('jobs.applications_error', 'Failed to load applications');
			}
		} catch (e) {
			applicationsError = e instanceof Error ? e.message : t('jobs.applications_error', 'Failed to load applications');
		} finally {
			applicationsLoading = false;
		}
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
				label: t('jobs.applications', 'Applications'),
				icon: Users,
				onclick: (row: JobListing) => openApplications(row)
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
			options: localizedStatusOptions
		}
	]);

	// ── Editable fields ─────
	// Core content fields (title/status/body) plus the canonical
	// job_listing_fields custom-field set. Custom fields are exposed under the
	// fv_ prefix — the load function flattens row.field_values onto each row so
	// EntityPage's row[field.key] seeding works, and update() maps them back
	// into the PATCH body's field_values map.
	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('jobs.title_col', 'Title'), type: 'text', required: true },
		{ key: 'status', label: t('jobs.status', 'Status'), type: 'select', options: localizedStatusOptions },
		{ key: 'body', label: t('jobs.body', 'Body'), type: 'textarea' },
		{ key: 'fv_company_name', label: t('jobs.company', 'Company'), type: 'text', required: true },
		{ key: 'fv_job_type', label: t('jobs.job_type', 'Job Type'), type: 'select', options: optionsFor('job_type'), required: true },
		{ key: 'fv_job_role', label: t('jobs.job_role', 'Job Role'), type: 'text' },
		{ key: 'fv_job_function', label: t('jobs.job_function', 'Job Function'), type: 'select', options: optionsFor('job_function') },
		{ key: 'fv_location_type', label: t('jobs.location_type', 'Location Type'), type: 'select', options: optionsFor('location_type'), required: true },
		{ key: 'fv_city', label: t('jobs.city', 'City'), type: 'text' },
		{ key: 'fv_country', label: t('jobs.country', 'Country'), type: 'text' },
		{ key: 'fv_salary_min', label: t('jobs.salary_min', 'Salary Min'), type: 'number' },
		{ key: 'fv_salary_max', label: t('jobs.salary_max', 'Salary Max'), type: 'number' },
		{ key: 'fv_salary_currency', label: t('jobs.salary_currency', 'Currency'), type: 'select', options: currencyOptions(currencies) },
		{ key: 'fv_salary_period', label: t('jobs.salary_period', 'Salary Period'), type: 'select', options: optionsFor('salary_period') },
		{ key: 'fv_experience_level', label: t('jobs.experience_level', 'Experience Level'), type: 'select', options: optionsFor('experience_level') },
		{ key: 'fv_skills_required', label: t('jobs.skills', 'Skills Required'), type: 'textarea' },
		{ key: 'fv_application_url', label: t('jobs.application_url', 'Application URL'), type: 'url' },
		{ key: 'fv_application_deadline', label: t('jobs.application_deadline', 'Application Deadline'), type: 'date' },
		{ key: 'fv_visa_sponsorship', label: t('jobs.visa_sponsorship', 'Visa Sponsorship'), type: 'checkbox' },
		{ key: 'fv_relocation_assistance', label: t('jobs.relocation_assistance', 'Relocation Assistance'), type: 'checkbox' },
		{ key: 'fv_company_website', label: t('jobs.company_website', 'Company Website'), type: 'url' },
		{ key: 'fv_company_logo', label: t('jobs.company_logo', 'Company Logo URL'), type: 'url' }
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
		// No POST /admin/jobs in the catalog — listings are org-created; the
		// admin surface moderates them. A Create button with no backing route
		// would submit into a dead handler.
		hideCreate: true,
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
					return `<span class="text-muted-foreground text-xs">${jobType ? optionLabel('job_type', jobType) : '—'}</span>`;
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
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${statusLabel(v)}</span>`;
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
		fields,
		load: async ({ q, filter, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			// EntityPage hands sort as "key:dir" — map to the backend's
			// ?sort=&sort_dir= admin-list convention.
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('sort_dir', sortDir);
			}
			params.set('page', String(page + 1));
			if (pageSize) params.set('limit', String(pageSize));
			const result = await jobsAdminApi.list(params.toString());
			if (result.ok) {
				const body = result.data;
				const listings = (body?.data as JobListing[]) ?? [];
				statusDist = body?.status_distribution ?? {};
				pageError = '';
				const total = body?.meta?.total ?? listings.length;
				const totalPages = total > 0 && pageSize ? Math.ceil(total / pageSize) : 1;
				// Flatten field_values onto each row as fv_<key> so the edit
				// form seeds them via row[field.key]; coerce boolean-typed
				// fields so checkbox bindings get real booleans.
				const data = listings.map((row) => {
					const flat: Record<string, unknown> = { ...row };
					for (const [k, v] of Object.entries(row.field_values ?? {})) {
						flat[`fv_${k}`] = BOOLEAN_FIELDS.has(k) ? v === 'true' : v;
					}
					return flat;
				});
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('jobs.load_error', 'Failed to load job listings'));
		},
		update: async (id, body) => {
			const { title, status, body: text, ...rest } = body as Record<string, unknown>;
			// Re-pack flattened fv_* form values into the PATCH body's
			// field_values map; booleans serialize back to 'true'/'false'.
			const fieldValues: Record<string, string> = {};
			for (const [k, v] of Object.entries(rest)) {
				if (!k.startsWith('fv_')) continue;
				fieldValues[k.slice(3)] = typeof v === 'boolean' ? String(v) : String(v ?? '');
			}
			// '' is never a valid §14 status — omit rather than persist a bogus
			// value when the select is cleared.
			const payload: Record<string, unknown> = { title, body: text, field_values: fieldValues };
			if (status) payload.status = status;
			const result = await jobsAdminApi.update(id, payload);
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
	<title>{t('jobs.title', 'Jobs Board')} — {siteName()} Admin</title>
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
					<span class="text-xs font-medium" style="color: var(--color-muted)">{statusLabel(status)}</span>
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
			<Star class="me-2 h-4 w-4" />
			{togglingId !== '' ? t('common.processing', 'Processing...') : t('common.confirm', 'Confirm')}
		</Button>
	</div>
</Modal>

<!-- Applications list for one listing (read-only per-job view) -->
<Modal bind:open={showApplications} title={t('jobs.applications', 'Applications')} size="lg">
	{#if applicationsTarget}
		<p class="mb-4 text-sm" style="color: var(--color-muted)">
			{t('jobs.applications_for', 'Applications for')} <strong>{applicationsTarget.title}</strong>
		</p>
	{/if}
	{#if applicationsLoading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.processing', 'Processing...')}</p>
	{:else if applicationsError}
		<p class="text-sm" style="color: var(--color-danger)">{applicationsError}</p>
	{:else if applications.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">{t('jobs.no_applications', 'No applications yet.')}</p>
	{:else}
		<div class="max-h-96 overflow-y-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b" style="border-color: var(--color-border)">
						<th class="px-2 py-2 text-left text-xs font-medium uppercase" style="color: var(--color-muted)">{t('jobs.applicant', 'Applicant')}</th>
						<th class="px-2 py-2 text-left text-xs font-medium uppercase" style="color: var(--color-muted)">{t('jobs.status', 'Status')}</th>
						<th class="px-2 py-2 text-left text-xs font-medium uppercase" style="color: var(--color-muted)">{t('jobs.applied_at', 'Applied')}</th>
					</tr>
				</thead>
				<tbody>
					{#each applications as app (app.id)}
						<tr class="border-b last:border-0" style="border-color: var(--color-border)">
							<td class="px-2 py-2" style="color: var(--color-foreground)">{app.applicant_username || '—'}</td>
							<td class="px-2 py-2"><span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">{app.status}</span></td>
							<td class="px-2 py-2" style="color: var(--color-muted)">{app.applied_at ? new Date(app.applied_at).toLocaleDateString() : '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showApplications = false)}>{t('common.close', 'Close')}</Button>
	</div>
</Modal>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet JobView({ row }: { row: JobListing })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{field(row, 'job_type') ? optionLabel('job_type', field(row, 'job_type')) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.location', 'Location')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{locationStr(row) || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.location_type', 'Location Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{field(row, 'location_type') ? optionLabel('location_type', field(row, 'location_type')) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{statusLabel(row.status)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.featured', 'Featured')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_featured ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.salary', 'Salary Range')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{#if field(row, 'salary_currency')}{currencySymbol(field(row, 'salary_currency'), currencies)} {/if}{[field(row, 'salary_min'), field(row, 'salary_max')].filter(Boolean).join('–') || '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.application_deadline', 'Application Deadline')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{field(row, 'application_deadline') || '—'}</p>
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
		{#if field(row, 'application_url')}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.application_url', 'Application URL')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{field(row, 'application_url')}</p>
			</div>
		{/if}
		{#if field(row, 'visa_sponsorship') === 'true'}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.visa_sponsorship', 'Visa Sponsorship')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{t('common.yes', 'Yes')}</p>
			</div>
		{/if}
		{#if field(row, 'relocation_assistance') === 'true'}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jobs.relocation_assistance', 'Relocation Assistance')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{t('common.yes', 'Yes')}</p>
			</div>
		{/if}
	</div>
{/snippet}
