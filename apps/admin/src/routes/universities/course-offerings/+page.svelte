<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /universities/course-offerings — per-university course offerings
	// (university_course_offerings, Section 39). EntityPage scaffold; the
	// course + university selects are hydrated from the API on mount (Rule 16 —
	// no hardcoded option lists).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { courseCatalogAdminApi, courseOfferingsAdminApi, universitiesAdminApi, type CourseOffering } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { loadCurrencies, currencyOptions, formatMoney, type CurrencyInfo } from '#lib/utils/currency.js';
	import { CheckCircle, EyeOff, Trash2 } from 'lucide-svelte';

	type SelectOption = { value: string; label: string };
	type AutocompleteItem = { id: string; label: string; sublabel?: string };

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkBusy = $state(false);
	// §58 — managed currency list for the currency select + tuition display.
	let currencies = $state<CurrencyInfo[]>([]);

	// ── Course/university pickers ─────────────────────────────────────────
	// API-backed Autocomplete fetchers (no 200-row cap — the query is sent
	// server-side) plus a label cache so selected chips render without a
	// re-fetch. The cache is seeded from list rows (each offering embeds
	// course.title + university.name) and from fetched options.
	const courseLabels = new Map<string, string>();
	const universityLabels = new Map<string, string>();

	const courseLabel = (id: string) => courseLabels.get(id) ?? id;
	const universityLabel = (id: string) => universityLabels.get(id) ?? id;

	async function fetchCourses(q: string): Promise<AutocompleteItem[]> {
		const params = new URLSearchParams({ limit: '25' });
		if (q.trim()) params.set('q', q.trim());
		const res = await courseCatalogAdminApi.list(params.toString());
		if (!res.ok) return [];
		return (res.data.data ?? []).map((c) => {
			courseLabels.set(c.id, c.title);
			return { id: c.id, label: c.title, sublabel: c.degree_type ?? undefined };
		});
	}

	async function fetchUniversities(q: string): Promise<AutocompleteItem[]> {
		const params = new URLSearchParams({ limit: '25' });
		if (q.trim()) params.set('q', q.trim());
		const res = await universitiesAdminApi.listPublic(params.toString());
		if (!res.ok) return [];
		// listPublic goes through api() which already unwraps the {data, meta}
		// envelope — res.data is the universities array, not the envelope.
		const rows = (Array.isArray(res.data) ? res.data : []) as { id: string; name: string; geo_code?: string }[];
		return rows.map((u) => {
			universityLabels.set(u.id, u.name);
			return { id: u.id, label: u.name, sublabel: u.geo_code ?? undefined };
		});
	}

	// Initial suggestions when the picker opens with an empty query.
	const initialCourses = () => fetchCourses('');
	const initialUniversities = () => fetchUniversities('');

	$effect(() => {
		loadCurrencies().then((list) => (currencies = list));
	});

	async function bulkAction(action: 'publish' | 'unpublish', ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkBusy) return;
		bulkBusy = true;
		pageError = '';
		try {
			const res = await courseOfferingsAdminApi.bulk(action, idArr);
			if (!res.ok) pageError = res.error ?? t('common.error', 'Bulk action failed');
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Bulk action failed');
		} finally {
			bulkBusy = false;
		}
	}

	// ── Bulk force delete — single-row force delete is handled by the
	// scaffold's built-in Hard Delete action (config.hardRemove). ──
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session so deleted-row filtering keeps working.
	const offeringRowMap = new Map<string, CourseOffering>();
	const offeringRowsFor = (ids: Set<string>): CourseOffering[] =>
		Array.from(ids)
			.map((id) => offeringRowMap.get(id))
			.filter((r): r is CourseOffering => !!r);

	let forceTargets = $state<CourseOffering[]>([]);
	let showForceModal = $state(false);

	function openBulkForceDelete(ids: Set<string>) {
		const rows = offeringRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (bulkBusy || forceTargets.length === 0) return;
		bulkBusy = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await courseOfferingsAdminApi.hardDelete(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulkDeletePartial', '{failed} of {n} items could not be deleted')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkBusy = false;
		}
	}

	function parseIntakeMonths(v: unknown): string[] | undefined {
		if (v === undefined || v === null) return undefined;
		if (Array.isArray(v)) return v;
		const months = String(v).split(',').map((m) => m.trim()).filter(Boolean);
		return months;
	}

	// ── Intake months ─────────────────────────────────────────────────────
	// Calendar months are a fixed canonical set (not tenant data) — the
	// stored values are English month names matching the seed format.
	const MONTH_OPTIONS: SelectOption[] = [
		'January', 'February', 'March', 'April', 'May', 'June',
		'July', 'August', 'September', 'October', 'November', 'December'
	].map((m) => ({ value: m, label: m }));

	// ── Accreditation ─────────────────────────────────────────────────────
	// Canonical accreditor suggestion list (same class as study_mode) —
	// freeText stays enabled for bodies outside the list.
	const ACCREDITORS = [
		'NUC accredited', 'NMCN accredited', 'COREN accredited',
		'AACSB', 'ABET', 'EQUIS', 'AMBA', 'QAA', 'NAAC',
		'WASC', 'SACSCOC', 'TEQSA', 'HLC', 'CAA'
	];
	const accreditorFetcher = async (q: string) => {
		const needle = q.trim().toLowerCase();
		return ACCREDITORS.filter((a) => !needle || a.toLowerCase().includes(needle))
			.map((a) => ({ id: a, label: a }));
	};
	const accreditorInitial = async () => ACCREDITORS.map((a) => ({ id: a, label: a }));

	const fields: FieldDef[] = $derived([
		{ key: 'course_id', label: t('courses.course', 'Course'), type: 'autocomplete', required: true,
			placeholder: t('courses.search_courses', 'Search courses...'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchCourses, initialFetcher: initialCourses, getSelectedLabel: courseLabel },
		{ key: 'university_id', label: t('courses.university', 'University'), type: 'autocomplete', required: true,
			placeholder: t('courses.search_universities', 'Search universities...'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchUniversities, initialFetcher: initialUniversities, getSelectedLabel: universityLabel },
		{ key: 'slug', label: t('courses.slug', 'Slug'), type: 'text', placeholder: t('courses.offering_slug_hint', 'Auto: {university}-{course} if empty') },
		{ key: 'tuition', label: t('courses.tuition', 'Tuition'), type: 'number' },
		currencies.length > 0
			? { key: 'currency', label: t('courses.currency', 'Currency'), type: 'select', options: currencyOptions(currencies) }
			: { key: 'currency', label: t('courses.currency', 'Currency'), type: 'text', placeholder: 'USD' },
		{ key: 'duration_years', label: t('courses.duration', 'Duration (years)'), type: 'number' },
		{
			key: 'study_mode', label: t('courses.study_mode', 'Study Mode'), type: 'select',
			options: [
				{ value: 'Full-time', label: t('courses.mode_fulltime', 'Full-time') },
				{ value: 'Part-time', label: t('courses.mode_parttime', 'Part-time') },
				{ value: 'Distance', label: t('courses.mode_distance', 'Distance') },
				{ value: 'Online', label: t('courses.mode_online', 'Online') },
				{ value: 'Hybrid', label: t('courses.mode_hybrid', 'Hybrid') }
			]
		},
		{ key: 'intake_months', label: t('courses.intake_months', 'Intake Months'), type: 'multiselect', options: MONTH_OPTIONS,
			placeholder: t('courses.intake_months_hint', 'Select intake months...') },
		{ key: 'entry_requirements', label: t('courses.entry_requirements', 'Entry Requirements'), type: 'textarea' },
		{ key: 'accreditation', label: t('courses.accreditation', 'Accreditation'), type: 'autocomplete',
			placeholder: t('courses.accreditation_hint', 'Search accreditors...'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: accreditorFetcher, initialFetcher: accreditorInitial,
			getSelectedLabel: (v) => v, freeText: true, allowFreeText: true },
		{ key: 'is_published', label: t('courses.published', 'Published'), type: 'checkbox' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('courses.offerings_title', 'Course Offerings'),
		description: t('courses.offerings_desc', 'Degree programs offered by each university — tuition, duration, intakes'),
		createLabel: t('courses.add_offering', 'Add Offering'),
		createPermission: 'universities.course_offering.create',
		editPermission: 'universities.course_offering.update',
		deletePermission: 'universities.course_offering.delete',
		// Fail-closed: hard delete needs the dedicated grant; without it the
		// scaffold hides the Hard Delete button entirely.
		hardDeletePermission: 'admin.hard_delete',
		pageSize: 25,
		columns: [
			{ key: 'course_title', label: t('courses.course', 'Course'), sortable: true, sortKey: 'title', render: (v: string) => `<span class="font-medium">${v ?? ''}</span>` },
			{ key: 'university_name', label: t('courses.university', 'University'), sortable: true, sortKey: 'university', render: (v: string) => `<span class="text-xs">${v ?? ''}</span>` },
			{ key: 'degree_type', label: t('courses.degree_type', 'Degree'), render: (v: string | null) => v ? `<span class="text-xs">${v}</span>` : '—' },
			{ key: 'tuition_display', label: t('courses.tuition', 'Tuition'), render: (v: string) => `<span class="text-xs">${v}</span>` },
			{ key: 'study_mode', label: t('courses.study_mode', 'Mode'), render: (v: string | null) => v ? `<span class="text-xs">${v}</span>` : '—' },
			{
				key: 'is_published',
				label: t('courses.status', 'Status'),
				render: (v: boolean, row: CourseOffering) => row.deleted_at
					? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-danger) 15%, transparent); color: var(--color-danger)">${t('common.filter_deleted', 'Deleted')}</span>`
					: v
					? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)">${t('courses.published', 'Published')}</span>`
					: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">${t('courses.unpublished', 'Unpublished')}</span>`
			}
		],
		filters: [
			{
				key: 'university_id',
				label: t('courses.university', 'University'),
				type: 'autocomplete',
				placeholder: t('courses.search_universities', 'Search universities...'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchUniversities,
				initialFetcher: initialUniversities,
				getSelectedLabel: universityLabel
			},
			{
				key: 'course_id',
				label: t('courses.course', 'Course'),
				type: 'autocomplete',
				placeholder: t('courses.search_courses', 'Search courses...'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchCourses,
				initialFetcher: initialCourses,
				getSelectedLabel: courseLabel
			},
			{
				key: 'is_published',
				label: t('courses.status', 'Status'),
				options: [
					{ value: 'true', label: t('courses.published', 'Published') },
					{ value: 'false', label: t('courses.unpublished', 'Unpublished') }
				]
			},
			// Soft-deleted offerings are hidden by default — this filter surfaces
			// them so the Hard Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		],
		bulkActions: [
			{
				label: t('courses.bulk_publish', 'Publish'),
				icon: CheckCircle,
				onclick: (ids) => bulkAction('publish', ids),
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			{
				label: t('courses.bulk_unpublish', 'Unpublish'),
				icon: EyeOff,
				onclick: (ids) => bulkAction('unpublish', ids),
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			// "Delete Selected" is the scaffold's built-in (confirm-gated,
			// per-row config.remove) — publish/unpublish use the §39 bulk
			// endpoint; only the custom force-delete stays besides them.
			{
				label: t('common.force_delete', 'Force Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openBulkForceDelete,
				disabled: (ids) => bulkBusy || !offeringRowsFor(ids).some((r) => !!r.deleted_at)
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter, sort }) => {
			const params = new URLSearchParams();
			params.set('page', String(page + 1));
			params.set('limit', String(pageSize));
			if (filter.university_id) params.set('university_id', filter.university_id);
			if (filter.course_id) params.set('course_id', filter.course_id);
			if (filter.is_published) params.set('is_published', filter.is_published);
			if (filter.deleted) params.set('deleted', filter.deleted);
			if (q) params.set('q', q);
			// DataTable emits the column sortKey ('title' | 'university') —
			// both map 1:1 onto the backend's ?sort= vocabulary.
			if (sort) {
				const [key] = sort.split(':');
				if (key === 'title') params.set('sort', 'title');
				else if (key === 'university') params.set('sort', 'university');
			}
			const result = await courseOfferingsAdminApi.list(params.toString());
			if (result.ok) {
				const rows = result.data.data ?? [];
				for (const r of rows) {
					offeringRowMap.set(r.id, r);
					// Seed the picker label caches from the joined names so edit
					// + filter chips render without an extra lookup.
					if (r.course?.title) courseLabels.set(r.course_id, r.course.title);
					if (r.university?.name) universityLabels.set(r.university_id, r.university.name);
				}
				const data = rows.map((o) => ({
					...o,
					course_title: o.course?.title ?? '',
					university_name: o.university?.name ?? '',
					degree_type: o.course?.degree_type ?? null,
					tuition_display: o.tuition != null ? formatMoney(o.tuition, o.currency, currencies) : '—'
				}));
				const total = result.data.meta?.total ?? data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load course offerings'));
		},
		create: async (body) => {
			const res = await courseOfferingsAdminApi.create({
				course_id: String(body.course_id ?? ''),
				university_id: String(body.university_id ?? ''),
				slug: String(body.slug ?? '').trim() || undefined,
				tuition: body.tuition ? Number(body.tuition) : undefined,
				currency: body.currency ? String(body.currency).trim() : undefined,
				duration_years: body.duration_years ? Number(body.duration_years) : undefined,
				study_mode: body.study_mode ? String(body.study_mode) : undefined,
				intake_months: parseIntakeMonths(body.intake_months),
				entry_requirements: body.entry_requirements ? String(body.entry_requirements) : undefined,
				accreditation: body.accreditation ? String(body.accreditation) : undefined,
				// EntityPage seeds checkbox fields to '' — '' is falsy so the
				// unchecked default (unpublished) is what the form shows.
				is_published: !!body.is_published
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// The backend performs a full-row update: send every field, with
			// null for cleared optional values.
			const res = await courseOfferingsAdminApi.update(id, {
				course_id: String(body.course_id ?? ''),
				university_id: String(body.university_id ?? ''),
				slug: String(body.slug ?? '').trim() || undefined,
				tuition: body.tuition === '' || body.tuition == null ? null : Number(body.tuition),
				currency: String(body.currency || '').trim() || null,
				duration_years: body.duration_years === '' || body.duration_years == null ? null : Number(body.duration_years),
				study_mode: String(body.study_mode || '') || null,
				intake_months: parseIntakeMonths(body.intake_months) ?? [],
				entry_requirements: String(body.entry_requirements || '') || null,
				accreditation: String(body.accreditation || '') || null,
				is_published: body.is_published === undefined ? undefined : !!body.is_published
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await courseOfferingsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		// Permanent removal — the scaffold shows "Hard Delete" only for rows
		// where isDeleted(row) is true (default: deleted_at != null).
		hardRemove: async (id) => {
			const res = await courseOfferingsAdminApi.hardDelete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: OfferingView
	});
</script>

<svelte:head>
	<title>{t('courses.offerings_title', 'Course Offerings')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Bulk Force Delete confirmation — permanent erasure of soft-deleted rows -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	{#if forceTargets.length > 0}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('common.force_delete_confirm_bulk', 'Permanently delete {count} selected items? This cannot be undone.').replace('{count}', String(forceTargets.length))}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showForceModal = false)} disabled={bulkBusy}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkBusy}>{bulkBusy ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}</Button>
	</div>
</Modal>

{#snippet OfferingView({ row }: { row: CourseOffering & { course_title?: string; university_name?: string } })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.course', 'Course')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.course?.title ?? row.course_title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.university', 'University')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.university?.name ?? row.university_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.slug', 'Slug')}</p>
			<p class="mt-1 font-mono text-sm" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.tuition', 'Tuition')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.tuition != null ? formatMoney(row.tuition, row.currency, currencies) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.duration', 'Duration')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.duration_years ? `${row.duration_years} yrs` : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.study_mode', 'Study Mode')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.study_mode ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.intake_months', 'Intake Months')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{Array.isArray(row.intake_months) && row.intake_months.length ? row.intake_months.join(', ') : '—'}</p>
			</div>
		</div>
		{#if row.entry_requirements}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.entry_requirements', 'Entry Requirements')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.entry_requirements}</p>
			</div>
		{/if}
		{#if row.accreditation}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.accreditation', 'Accreditation')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.accreditation}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.status', 'Status')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_published ? t('courses.published', 'Published') : t('courses.unpublished', 'Unpublished')}</p>
		</div>
	</div>
{/snippet}
