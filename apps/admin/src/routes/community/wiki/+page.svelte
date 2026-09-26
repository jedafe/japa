<script lang="ts">
	// /community/wiki — admin HITL review queue for wiki page revisions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Tabs } from '$lib/components/ui/tabs';
	import RevisionDiff from '$lib/components/wiki/RevisionDiff.svelte';
	import WikiTypesAdmin from '$lib/components/wiki/WikiTypesAdmin.svelte';
	import { wikiAdminApi, apiRaw, type WikiRevisionItem } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Sparkles, Lock, Unlock, Star, FileDiff } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type WikiItem = {
		id: string;
		title: string;
		space_slug: string;
		status: string;
		updated_at: string;
		latest_revision_id?: string;
		excerpt?: string;
		is_locked?: boolean;
		is_featured?: boolean;
		is_active?: boolean;
		data_entity_type?: string | null;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canReview = $derived(hasPermission('wiki.review'));
	const canManage = $derived(hasPermission('wiki.manage'));
	const canLock = $derived(hasPermission('wiki.lock'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── View switcher: Pages (existing page-scoped queue) vs Revisions (§80
	// flat revision queue with kind/status filters + field-ops diff) vs
	// Types (§80 Wiki Types — managed types + category templates) ──────────
	let view = $state<'pages' | 'revisions' | 'types'>('pages');

	// ── State-action confirmation modal (reject with optional reason) ────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let revisionsController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<WikiItem | null>(null);
	let revisionActionTarget = $state<WikiRevisionItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

	const rejectTargetLabel = $derived(
		actionTarget?.title ?? revisionActionTarget?.summary ?? revisionActionTarget?.id ?? ''
	);

	function openReject(item: WikiItem) {
		actionTarget = item;
		revisionActionTarget = null;
		rejectReason = '';
		showRejectModal = true;
	}

	function openRevisionReject(rev: WikiRevisionItem) {
		revisionActionTarget = rev;
		actionTarget = null;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			if (revisionActionTarget) {
				// Revision-keyed route (§80): works for page-less ops revisions too.
				const result = await wikiAdminApi.rejectWikiRevision(revisionActionTarget.id, { reason: rejectReason });
				if (result.ok) {
					showRejectModal = false;
					revisionsController?.reload();
				} else {
					pageError = result.error ?? t('common.error', 'An error occurred');
				}
			} else if (actionTarget?.latest_revision_id) {
				const result = await wikiAdminApi.rejectRevision(actionTarget.id, actionTarget.latest_revision_id, { reason: rejectReason });
				if (result.ok) {
					showRejectModal = false;
					entityController?.reload();
				} else {
					pageError = result.error ?? t('common.error', 'An error occurred');
				}
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function approveRevision(item: WikiItem) {
		if (!item.latest_revision_id || acting) return;
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.approveRevision(item.id, item.latest_revision_id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// Revision-keyed approve (§80): required for fields/mixed revisions, which
	// may target an entity without a host wiki page.
	async function approveRevisionById(rev: WikiRevisionItem) {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.approveWikiRevision(rev.id);
			if (result.ok) {
				revisionsController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Page management: lock/unlock/feature (wiki.manage / wiki.lock) ──────
	async function toggleLock(item: WikiItem) {
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.update(item.id, { is_locked: !item.is_locked });
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function toggleFeatured(item: WikiItem) {
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.update(item.id, { is_featured: !item.is_featured });
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function toggleActive(item: WikiItem) {
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.update(item.id, { is_active: !(item.is_active ?? true) });
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// data_entity_type binds the page to a child entity class (e.g.
	// 'visa_route') so §80 field ops target that entity — PATCHed from the
	// view slide-over (wiki.manage).
	let detSaving = $state(false);
	let detDraft = $state('');
	let detRowId = $state('');

	function detValue(row: WikiItem): string {
		return detRowId === row.id ? detDraft : (row.data_entity_type ?? '');
	}

	async function saveDataEntityType(row: WikiItem) {
		if (detSaving) return;
		detSaving = true;
		pageError = '';
		try {
			const v = detValue(row).trim();
			// NULLIF('') clears the binding server-side; JSON null is "omit".
			const result = await wikiAdminApi.update(row.id, { data_entity_type: v });
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			detSaving = false;
		}
	}

	// ── AI generate draft modal (wiki.manage) ───────────────────────────────
	let showGenerateModal = $state(false);
	let genSpaceSlug = $state('');
	let genPageTitle = $state('');
	let genPageSlug = $state('');
	let genSlugTouched = $state(false);
	let genCategory = $state('overview');
	let genCategories = $state<Array<{ value: string; label: string }>>([]);
	let generating = $state(false);

	function slugify(s: string): string {
		return s
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	function onGenTitleInput(e: Event) {
		genPageTitle = (e.currentTarget as HTMLInputElement).value;
		if (!genSlugTouched) genPageSlug = slugify(genPageTitle);
	}

	function onGenSlugInput(e: Event) {
		genSlugTouched = true;
		genPageSlug = slugify((e.currentTarget as HTMLInputElement).value);
	}

	// Canonical category list for the space (university spaces get a
	// different set — GET /wiki/categories?space= resolves that server-side).
	async function loadGenCategories(spaceSlug: string) {
		const qs = spaceSlug ? `?space=${encodeURIComponent(spaceSlug)}` : '';
		const res = await apiRaw<{ data?: Array<{ slug?: string; label_key?: string; label?: string }> }>(
			`/wiki/categories${qs}`
		);
		if (res.ok && Array.isArray(res.data?.data)) {
			genCategories = res.data.data.map((c) => ({
				value: c.slug ?? '',
				label: c.label_key ? t(c.label_key, c.label ?? c.slug ?? '') : (c.label ?? c.slug ?? '')
			})).filter((c) => c.value);
		}
	}

	function openGenerate() {
		genSpaceSlug = '';
		genPageTitle = '';
		genPageSlug = '';
		genSlugTouched = false;
		genCategory = 'overview';
		genCategories = [];
		showGenerateModal = true;
		void loadGenCategories('');
	}

	async function onGenSpaceChange() {
		genCategory = 'overview';
		await loadGenCategories(genSpaceSlug.trim());
	}

	async function confirmGenerate() {
		if (!genSpaceSlug.trim() || !genPageTitle.trim() || !genPageSlug.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.generate({
				space_slug: genSpaceSlug.trim(),
				page_slug: genPageSlug.trim(),
				title: genPageTitle.trim(),
				category: genCategory
			});
			if (result.ok) {
				showGenerateModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			generating = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: WikiItem, ctx) => ctx?.openView(_row) }
		];
		if (canReview) {
			acts.push({
				label: t('app.wiki.approve_revision', 'Approve Revision'),
				icon: CheckCircle,
				onclick: approveRevision,
				show: (row: WikiItem) => !!row.latest_revision_id
			});
			acts.push({
				label: t('app.wiki.reject_revision', 'Reject Revision'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: WikiItem) => !!row.latest_revision_id
			});
		}
		if (canLock) {
			acts.push({
				label: t('app.wiki.toggle_lock', 'Toggle Lock'),
				icon: Lock,
				onclick: toggleLock
			});
		}
		if (canManage) {
			acts.push({
				label: t('app.wiki.toggle_featured', 'Toggle Featured'),
				icon: Star,
				onclick: toggleFeatured
			});
			acts.push({
				label: t('app.wiki.toggle_active', 'Toggle Active'),
				icon: CheckCircle,
				onclick: toggleActive
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.wiki.col_title', 'Title'), type: 'text' },
		{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), type: 'text' },
		{ key: 'status', label: t('app.wiki.col_status', 'Status'), type: 'text' },
		{ key: 'excerpt', label: t('app.wiki.detail_excerpt', 'Excerpt'), type: 'textarea' }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'dest',
				label: t('app.wiki.col_destination', 'Destination'),
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
		filters,
		title: t('app.wiki.review_queue_heading', 'Wiki Review Queue'),
		description: t('app.wiki.review_queue_desc', 'Review and approve pending wiki page revisions'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.wiki.col_title', 'Title'), sortable: true },
			{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), sortable: true },
			{ key: 'status', label: t('app.wiki.col_status', 'Status'), sortable: true },
			{
				key: 'ai_generated',
				label: t('app.wiki.col_ai', 'AI'),
				sortable: false,
				render: (value: boolean) => (value ? '🤖' : '—')
			},
			{
				key: 'is_locked',
				label: t('app.wiki.col_locked', 'Locked'),
				sortable: true,
				render: (value: boolean) => (value ? '🔒' : '—')
			},
			{
				key: 'updated_at',
				label: t('app.wiki.col_updated', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			// EntityPage's params.page is 0-based; the backend page param is 1-based.
			qs.set('page', String(params.page + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			const result = await apiRaw<{ data: WikiItem[]; pagination: { total: number } }>(
				`/admin/wiki${qs.toString() ? `?${qs.toString()}` : ''}`
			);
			if (result.ok) {
				const body = result.data as { data: WikiItem[]; pagination: { total: number } };
				const data = body.data ?? [];
				const total = body.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: WikiView
	});

	// ── Revisions queue (§80): flat list from GET /admin/wiki/revisions ─────
	// Content revisions belong to a page; fields/mixed revisions carry an ops
	// array and may be page-less. Approve/reject use the revision-keyed routes.

	const revisionFilters = $derived.by<FilterDef[]>(() => [
		{
			key: 'kind',
			label: t('app.wiki.filter_kind', 'Kind'),
			type: 'select',
			options: [
				{ value: 'all', label: t('common.all', 'All') },
				{ value: 'content', label: t('app.wiki.kind_content', 'Content') },
				{ value: 'fields', label: t('app.wiki.kind_fields', 'Fields') },
				{ value: 'mixed', label: t('app.wiki.kind_mixed', 'Mixed') }
			]
		},
		{
			key: 'status',
			label: t('app.wiki.col_status', 'Status'),
			type: 'select',
			options: [
				{ value: 'all', label: t('common.all', 'All') },
				{ value: 'pending', label: t('app.wiki.status_pending', 'Pending') },
				{ value: 'approved', label: t('app.wiki.status_approved', 'Approved') },
				{ value: 'rejected', label: t('app.wiki.status_rejected', 'Rejected') }
			]
		}
	]);

	const revisionActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{
				label: t('app.wiki.view_ops', 'View Ops'),
				icon: FileDiff,
				onclick: (row: WikiRevisionItem, ctx) => ctx?.openView(row),
				show: (row: WikiRevisionItem) => row.kind !== 'content'
			},
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row: WikiRevisionItem, ctx) => ctx?.openView(row),
				show: (row: WikiRevisionItem) => row.kind === 'content'
			}
		];
		if (canReview) {
			acts.push({
				label: t('app.wiki.approve_revision', 'Approve Revision'),
				icon: CheckCircle,
				onclick: approveRevisionById,
				show: (row: WikiRevisionItem) => row.status === 'pending'
			});
			acts.push({
				label: t('app.wiki.reject_revision', 'Reject Revision'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openRevisionReject,
				show: (row: WikiRevisionItem) => row.status === 'pending'
			});
		}
		return acts;
	});

	// Minimal FieldDef set — EntityPage requires `fields` even for readonly pages.
	const revisionFields: FieldDef[] = [
		{ key: 'summary', label: t('app.wiki.col_summary', 'Summary'), type: 'text' }
	];

	const revisionConfig = $derived<EntityPageConfig>({
		title: t('app.wiki.revisions_heading', 'Revision Queue'),
		description: t('app.wiki.revisions_desc', 'Review pending wiki revisions, including community field ops'),
		readonly: true,
		pageSize: 25,
		filters: revisionFilters,
		columns: [
			{ key: 'kind', label: t('app.wiki.col_kind', 'Kind'), sortable: false },
			{
				key: 'summary',
				label: t('app.wiki.col_summary', 'Summary'),
				sortable: false,
				render: (value: string) => value || '—'
			},
			{
				key: 'target_entity_type',
				label: t('app.wiki.col_target', 'Target'),
				sortable: false,
				render: (_value: string, row: WikiRevisionItem) =>
					row.target_entity_type
						? `${row.target_entity_type} · ${(row.target_entity_id ?? '').slice(0, 8)}`
						: '—'
			},
			{
				key: 'submitted_by',
				label: t('app.wiki.col_submitted_by', 'Submitted By'),
				sortable: false,
				render: (value: string) => value || '—'
			},
			{ key: 'status', label: t('app.wiki.col_status', 'Status'), sortable: false },
			{
				key: 'created_at',
				label: t('app.wiki.col_created', 'Submitted'),
				sortable: false,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: revisionActions,
		fields: revisionFields,
		load: async (params) => {
			const qs = new URLSearchParams();
			// EntityPage's params.page is 0-based; the backend page param is 1-based.
			qs.set('page', String(params.page + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			const kind = params.filter?.kind;
			if (kind && kind !== 'all') qs.set('kind', kind);
			const status = params.filter?.status;
			if (status && status !== 'all') qs.set('status', status);
			const result = await wikiAdminApi.listWikiRevisions(qs.toString());
			if (result.ok) {
				const data = result.data.data ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: RevisionView
	});

	// Revision content is stored as a TipTap JSON doc (or a plain JSON string);
	// extract the text nodes for a readable preview in the view slide-over.
	function extractContentText(content: unknown): string {
		if (content == null) return '';
		if (typeof content === 'string') return content;
		const parts: string[] = [];
		const walk = (node: unknown) => {
			if (!node || typeof node !== 'object') return;
			const n = node as Record<string, unknown>;
			if (typeof n.text === 'string') parts.push(n.text);
			if (Array.isArray(n.content)) n.content.forEach(walk);
		};
		walk(content);
		return parts.join(' ');
	}
</script>

<svelte:head>
	<title>{t('app.wiki.review_queue_page_title', 'Wiki Review Queue — JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<Tabs
	class="mb-4"
	tabs={[
		{ value: 'pages', label: t('app.wiki.tab_pages', 'Pages') },
		{ value: 'revisions', label: t('app.wiki.tab_revisions', 'Revisions') },
		...(canManage ? [{ value: 'types', label: t('app.wiki.tab_types', 'Types & Categories') }] : [])
	]}
	bind:value={view}
/>

{#if view === 'pages'}
	{#if canManage}
		<div class="mb-4 flex justify-end">
			<Button onclick={openGenerate}>
				<Sparkles size={16} class="mr-1.5" />
				{t('app.wiki.generate_draft', 'Generate AI Draft')}
			</Button>
		</div>
	{/if}

	<EntityPage {config} bind:controller={entityController} />
{:else if view === 'revisions'}
	<EntityPage config={revisionConfig} bind:controller={revisionsController} />
{:else}
	<WikiTypesAdmin />
{/if}

<!-- ── Reject revision confirmation modal ─────────────────────────────────── -->
<Modal bind:open={showRejectModal} title={t('app.wiki.modal_reject_revision', 'Reject Revision')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.wiki.reject_confirm', 'Are you sure you want to reject the revision for "{title}"?').replace('{title}', rejectTargetLabel)}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.reason_optional', 'Reason (optional)')}</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={rejectReason}
				placeholder={t('app.wiki.reject_placeholder', 'Explain why this revision was rejected...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.wiki.rejecting', 'Rejecting...') : t('app.wiki.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- ── AI generate draft modal (wiki.manage) ─────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.wiki.modal_generate_draft', 'Generate AI Draft')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.wiki.generate_desc', 'Generate a wiki page draft using the Content Agent. The draft will be created as a pending revision for HITL review.')}
		</p>
		<div>
			<label for="gen-space-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.gen_space_slug', 'Space slug')}</label>
			<input
				id="gen-space-slug"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="e.g. japan"
				bind:value={genSpaceSlug}
				onblur={onGenSpaceChange}
			/>
		</div>
		<div>
			<label for="gen-page-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.gen_page_title', 'Page title')}</label>
			<input
				id="gen-page-title"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="e.g. Visa Routes"
				value={genPageTitle}
				oninput={onGenTitleInput}
			/>
		</div>
		<div>
			<label for="gen-page-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.gen_page_slug', 'Page slug')}</label>
			<input
				id="gen-page-slug"
				type="text"
				class="w-full rounded-lg border p-3 font-mono text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="e.g. visa-routes"
				value={genPageSlug}
				oninput={onGenSlugInput}
			/>
		</div>
		<div>
			<label for="gen-category" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.gen_category', 'Category')}</label>
			<select
				id="gen-category"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={genCategory}
			>
				{#if genCategories.length === 0}
					<option value="overview">{t('wiki.category_overview', 'Overview')}</option>
				{:else}
					{#each genCategories as c (c.value)}
						<option value={c.value}>{c.label}</option>
					{/each}
				{/if}
			</select>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !genSpaceSlug.trim() || !genPageTitle.trim() || !genPageSlug.trim()}>
			{generating ? t('app.wiki.generating', 'Generating...') : t('app.wiki.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet WikiView({ row }: { row: WikiItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_space', 'Space')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.space_slug || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_updated', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.detail_excerpt', 'Excerpt')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.excerpt || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_ai', 'AI-generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.ai_generated ? `${t('app.wiki.ai_yes', 'Yes')} (${row.ai_model ?? '—'})` : t('app.wiki.ai_no', 'No')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_locked', 'Locked')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_locked ? t('app.wiki.locked_yes', 'Yes') : t('app.wiki.locked_no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_featured', 'Featured')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_featured ? t('app.wiki.featured_yes', 'Yes') : t('app.wiki.featured_no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(row.is_active ?? true) ? t('app.wiki.active_yes', 'Yes') : t('app.wiki.active_no', 'No')}</p>
		</div>
		{#if canManage}
			<div>
				<label
					for="det-input-{row.id}"
					class="mb-1 block text-xs font-medium uppercase tracking-wider"
					style="color: var(--color-muted)"
				>
					{t('app.wiki.data_entity_type', 'Data entity type')}
				</label>
				<div class="flex gap-2">
					<input
						id="det-input-{row.id}"
						type="text"
						class="min-w-0 flex-1 rounded-lg border p-2 font-mono text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						placeholder="e.g. visa_route"
						value={detValue(row)}
						oninput={(e) => {
							detRowId = row.id;
							detDraft = e.currentTarget.value;
						}}
					/>
					<Button variant="ghost" disabled={detSaving} onclick={() => saveDataEntityType(row)}>
						{detSaving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</Button>
				</div>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					{t('app.wiki.data_entity_type_hint', 'Binds this page to a child entity class for community field ops. Empty clears the binding.')}
				</p>
			</div>
		{/if}
	</div>
{/snippet}

<!-- ── Revision view slide-over body (revisions queue) ───────────────────── -->
{#snippet RevisionView({ row }: { row: WikiRevisionItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_kind', 'Kind')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.kind || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_summary', 'Summary')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_submitted_by', 'Submitted By')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.submitted_by || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_created', 'Submitted')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.reviewed_by || row.reviewed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_reviewed_by', 'Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.reviewed_by || '—'}{row.reviewed_at ? ` · ${new Date(row.reviewed_at).toLocaleString()}` : ''}
				</p>
			</div>
		{/if}
		{#if row.apply_error}
			<div
				class="rounded-lg border p-3 text-sm"
				style="border-color: var(--color-danger, #dc2626); background: var(--color-danger-bg, rgba(220,38,38,0.08)); color: var(--color-foreground)"
			>
				<p class="font-medium">{t('app.wiki.apply_error', 'Last apply attempt failed')}</p>
				<p class="mt-1" style="color: var(--color-muted)">{row.apply_error}</p>
			</div>
		{/if}
		{#if row.kind !== 'fields'}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.detail_content', 'Content')}</p>
				<pre
					class="mt-1 max-h-64 overflow-y-auto whitespace-pre-wrap rounded-lg border p-3 text-xs"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				>{extractContentText(row.content) || '—'}</pre>
			</div>
		{/if}
		{#if row.kind !== 'content'}
			<div>
				<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.detail_field_ops', 'Field Ops')}</p>
				<RevisionDiff revision={row} />
			</div>
		{/if}
	</div>
{/snippet}
