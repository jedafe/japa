<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// /community/wiki — admin HITL review queue for wiki page revisions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Tabs } from '$lib/components/ui/tabs';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import RevisionDiff from '$lib/components/wiki/RevisionDiff.svelte';
	import WikiTypesAdmin from '$lib/components/wiki/WikiTypesAdmin.svelte';
	import { wikiAdminApi, apiRaw, type WikiRevisionItem } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { initTabParam } from '$lib/utils/url-sync.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission, isPermissionsLoaded } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Sparkles, Lock, LockOpen, Star, StarOff, FileDiff, Trash2 } from 'lucide-svelte';
	import type { ActionItem, BulkAction, FilterDef } from '$lib/components/ui/data-table';

	// One row of GET /admin/wiki (AdminWikiPageDTO). status is the LATEST
	// revision's status ('' when the page has no revisions); ai_generated /
	// ai_model carry that revision's ADR-088 provenance. field_group_*
	// identify the page's bound custom field group (§80).
	type WikiItem = {
		id: string;
		slug: string;
		title: string;
		category: string;
		space_slug: string;
		space_name?: string;
		status: string;
		updated_at: string;
		latest_revision_id?: string;
		excerpt?: string;
		is_locked?: boolean;
		is_featured?: boolean;
		is_active?: boolean;
		created_at?: string;
		deleted_at?: string | null;
		data_entity_type?: string | null;
		page_template_id?: string;
		field_group_id?: string;
		field_group_slug?: string;
		field_group_name?: string;
		// ADR-088 AI provenance of the latest revision
		ai_generated?: boolean;
		ai_model?: string;
	};

	const canReview = $derived(hasPermission('wiki.review'));
	const canManage = $derived(hasPermission('wiki.manage'));
	const canLock = $derived(hasPermission('wiki.lock'));
	const canDelete = $derived(hasPermission('wiki.delete'));
	// ?force=true hard deletes require BOTH the resource delete permission
	// (RequirePermission runs first) and admin.hard_delete
	// (RequireForceDelete) — mirror the media/calendar pages' gating so
	// users without the hard-delete grant don't see actions that 403.
	const canHardDelete = $derived(canDelete && hasPermission('admin.hard_delete'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── View switcher: Pages (existing page-scoped queue) vs Revisions (§80
	// flat revision queue with kind/status filters + field-ops diff) vs
	// Types (§80 Wiki Types — managed types + category templates) ──────────
	// The active tab is deep-linkable via ?tab= (handled by the shared Tabs
	// component's tabParam prop; the 'types' tab is permission-gated so it is
	// validated against the tabs array once permissions resolve). List state
	// (q/page/sort/filters) is per-tab, so tabPreserve=[] wipes it on switch.
	let view = $state<'pages' | 'revisions' | 'types'>(
		initTabParam('pages', { valid: ['pages', 'revisions', 'types'] })
	);

	// 'types' is wiki.manage-gated — deep links to it degrade to 'pages'
	// once permissions resolve (don't reset earlier or a valid deep link
	// would bounce while the permission store is still loading).
	$effect(() => {
		if (isPermissionsLoaded() && view === 'types' && !canManage) {
			view = 'pages';
		}
	});

	// ── State-action confirmation modal (reject with optional reason) ────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let revisionsController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<WikiItem | null>(null);
	let revisionActionTarget = $state<WikiRevisionItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');
	// Bulk-reject path: rows captured at click time so the modal can outlive
	// the table selection (EntityPage clears it when the onclick returns).
	let bulkReject = $state<
		{ kind: 'page'; rows: WikiItem[] } | { kind: 'revision'; rows: WikiRevisionItem[] } | null
	>(null);

	const rejectTargetLabel = $derived(
		actionTarget?.title ?? revisionActionTarget?.summary ?? revisionActionTarget?.id ?? ''
	);

	function openReject(item: WikiItem) {
		actionTarget = item;
		revisionActionTarget = null;
		bulkReject = null;
		rejectReason = '';
		showRejectModal = true;
	}

	function openRevisionReject(rev: WikiRevisionItem) {
		revisionActionTarget = rev;
		actionTarget = null;
		bulkReject = null;
		rejectReason = '';
		showRejectModal = true;
	}

	function bulkPartialError(failed: number, total: number, lastErr: string) {
		pageError =
			t('app.wiki.bulk_partial', '{failed} of {n} items could not be completed')
				.replace('{failed}', String(failed))
				.replace('{n}', String(total)) + (lastErr ? ` — ${lastErr}` : '');
	}

	async function confirmReject() {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			if (bulkReject) {
				// Bulk path: same shared reason applied to every captured row.
				const target = bulkReject;
				let failed = 0;
				let lastErr = '';
				for (const row of target.rows) {
					const res =
						target.kind === 'page'
							? await wikiAdminApi.rejectRevision(row.id, (row as WikiItem).latest_revision_id ?? '', {
									reason: rejectReason
								})
							: await wikiAdminApi.rejectWikiRevision(row.id, { reason: rejectReason });
					if (!res.ok) {
						failed++;
						lastErr = res.error ?? '';
					}
				}
				showRejectModal = false;
				bulkReject = null;
				if (failed > 0) bulkPartialError(failed, target.rows.length, lastErr);
				// The scaffold already reloaded when the bulk onclick returned;
				// reload again so post-reject state is reflected.
				(target.kind === 'page' ? entityController : revisionsController)?.reload();
			} else if (revisionActionTarget) {
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
		if (acting) return;
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
		if (acting) return;
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
		if (acting) return;
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

	// ── Bulk actions ──────────────────────────────────────────────────────
	// BulkAction.onclick receives only ids, and BulkBar's selectedRows covers
	// just the current page — resolve ids against a map of every row seen this
	// session so cross-page selections (and approve's latest_revision_id)
	// keep working.
	const pageRowMap = new Map<string, WikiItem>();
	const revisionRowMap = new Map<string, WikiRevisionItem>();

	const pageRowsFor = (ids: Set<string>): WikiItem[] =>
		Array.from(ids)
			.map((id) => pageRowMap.get(id))
			.filter((r): r is WikiItem => !!r);
	const revisionRowsFor = (ids: Set<string>): WikiRevisionItem[] =>
		Array.from(ids)
			.map((id) => revisionRowMap.get(id))
			.filter((r): r is WikiRevisionItem => !!r);
	const approvablePages = (ids: Set<string>): WikiItem[] =>
		pageRowsFor(ids).filter((r) => r.status === 'pending' && !!r.latest_revision_id);
	const pendingRevisions = (ids: Set<string>): WikiRevisionItem[] =>
		revisionRowsFor(ids).filter((r) => r.status === 'pending' && !r.deleted_at);

	// Sequential per-row ops — the API has no bulk routes, and per-row failures
	// are collected rather than aborting the batch (same rule as EntityPage's
	// built-in bulk delete). The scaffold reloads + clears the selection after
	// this resolves.
	async function bulkEach<T extends { id: string }>(
		rows: T[],
		fn: (row: T) => Promise<{ ok: boolean; error?: string }>
	) {
		if (acting || rows.length === 0) return;
		acting = true;
		pageError = '';
		try {
			let failed = 0;
			let lastErr = '';
			for (const row of rows) {
				const res = await fn(row);
				if (!res?.ok) {
					failed++;
					lastErr = res?.error ?? '';
				}
			}
			if (failed > 0) bulkPartialError(failed, rows.length, lastErr);
		} finally {
			acting = false;
		}
	}

	function openBulkRejectPages(ids: Set<string>) {
		const rows = approvablePages(ids);
		if (rows.length === 0) return;
		bulkReject = { kind: 'page', rows };
		actionTarget = null;
		revisionActionTarget = null;
		rejectReason = '';
		showRejectModal = true;
	}

	function openBulkRejectRevisions(ids: Set<string>) {
		const rows = pendingRevisions(ids);
		if (rows.length === 0) return;
		bulkReject = { kind: 'revision', rows };
		actionTarget = null;
		revisionActionTarget = null;
		rejectReason = '';
		showRejectModal = true;
	}

	// ── Force delete (wiki.delete) — single + bulk share one confirm modal ──
	// Targets are captured at click time so the modal can outlive the table
	// selection (EntityPage clears it when the bulk onclick returns).
	let forceTargets = $state<WikiItem[]>([]);
	let showForceModal = $state(false);

	function openForceDelete(row: WikiItem) {
		forceTargets = [row];
		showForceModal = true;
	}

	function openBulkForceDelete(ids: Set<string>) {
		const rows = pageRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (acting || forceTargets.length === 0) return;
		acting = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await wikiAdminApi.hardDeletePage(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) bulkPartialError(failed, targets.length, lastErr);
			entityController?.reload();
		} finally {
			acting = false;
		}
	}

	// ── Revision force delete — same single+bulk modal pattern as pages ────
	let revForceTargets = $state<WikiRevisionItem[]>([]);
	let showRevForceModal = $state(false);

	function openRevisionForceDelete(row: WikiRevisionItem) {
		revForceTargets = [row];
		showRevForceModal = true;
	}

	function openBulkForceDeleteRevisions(ids: Set<string>) {
		const rows = revisionRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		revForceTargets = rows;
		showRevForceModal = true;
	}

	async function confirmRevisionForceDelete() {
		if (acting || revForceTargets.length === 0) return;
		acting = true;
		pageError = '';
		try {
			const targets = revForceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await wikiAdminApi.hardDeleteWikiRevision(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showRevForceModal = false;
			revForceTargets = [];
			if (failed > 0) bulkPartialError(failed, targets.length, lastErr);
			revisionsController?.reload();
		} finally {
			acting = false;
		}
	}

	const pageBulkActions = $derived.by<BulkAction[]>(() => {
		const acts: BulkAction[] = [];
		if (canReview) {
			acts.push(
				{
					label: t('app.wiki.bulk_approve', 'Approve Selected'),
					icon: CheckCircle,
					onclick: (ids) =>
						bulkEach(approvablePages(ids), (r) =>
							wikiAdminApi.approveRevision(r.id, r.latest_revision_id!)
						),
					disabled: (ids) => acting || approvablePages(ids).length === 0
				},
				{
					label: t('app.wiki.bulk_reject', 'Reject Selected'),
					icon: XCircle,
					variant: 'destructive',
					onclick: openBulkRejectPages,
					disabled: (ids) => acting || approvablePages(ids).length === 0
				}
			);
		}
		if (canManage && canLock) {
			acts.push(
				{
					label: t('app.wiki.bulk_lock', 'Lock Selected'),
					icon: Lock,
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => !r.is_locked),
							(r) => wikiAdminApi.update(r.id, { is_locked: true })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !r.is_locked)
				},
				{
					label: t('app.wiki.bulk_unlock', 'Unlock Selected'),
					icon: LockOpen,
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => !!r.is_locked),
							(r) => wikiAdminApi.update(r.id, { is_locked: false })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !!r.is_locked)
				}
			);
		}
		if (canManage) {
			acts.push(
				{
					label: t('app.wiki.bulk_feature', 'Feature Selected'),
					icon: Star,
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => !r.is_featured),
							(r) => wikiAdminApi.update(r.id, { is_featured: true })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !r.is_featured)
				},
				{
					label: t('app.wiki.bulk_unfeature', 'Unfeature Selected'),
					icon: StarOff,
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => !!r.is_featured),
							(r) => wikiAdminApi.update(r.id, { is_featured: false })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !!r.is_featured)
				},
				{
					label: t('app.wiki.bulk_activate', 'Activate Selected'),
					icon: CheckCircle,
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => !(r.is_active ?? true)),
							(r) => wikiAdminApi.update(r.id, { is_active: true })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !(r.is_active ?? true))
				},
				{
					label: t('app.wiki.bulk_deactivate', 'Deactivate Selected'),
					icon: XCircle,
					variant: 'destructive',
					onclick: (ids) =>
						bulkEach(
							pageRowsFor(ids).filter((r) => r.is_active ?? true),
							(r) => wikiAdminApi.update(r.id, { is_active: false })
						),
					disabled: (ids) => acting || !pageRowsFor(ids).some((r) => r.is_active ?? true)
				}
			);
		}
		if (canHardDelete) {
			// Permanent removal — only for already-soft-deleted rows (the
			// built-in "Delete Selected" above covers the soft delete), and
			// gated on admin.hard_delete (backend RequireForceDelete).
			acts.push({
				label: t('app.wiki.bulk_force_delete', 'Force Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openBulkForceDelete,
				disabled: (ids) => acting || !pageRowsFor(ids).some((r) => !!r.deleted_at)
			});
		}
		return acts;
	});

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
	let genSpaceLabel = $state('');
	let genPageTitle = $state('');
	let genPageSlug = $state('');
	let genSlugTouched = $state(false);
	let genCategory = $state('overview');
	let genCategoryLabel = $state('');
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

	// Destination space picker — GET /wiki/destinations?q= searches active
	// spaces; has_wiki=false so brand-new spaces (no pages yet) are included.
	async function genSpaceFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await apiRaw<{ data?: Array<{ slug: string; name: string; level?: string }> }>(
			`/wiki/destinations?q=${encodeURIComponent(q)}&has_wiki=false`
		);
		if (!res.ok || !Array.isArray(res.data?.data)) return [];
		return res.data.data.map((d) => ({ id: d.slug, label: d.name, sublabel: d.level ?? '' }));
	}

	async function selectGenSpace(item: AutocompleteItem) {
		genSpaceSlug = item.id;
		genSpaceLabel = item.label;
		await loadGenCategories(item.id);
		// Re-default when the previous pick isn't in the space's template.
		if (!genCategories.some((c) => c.value === genCategory)) {
			genCategory = genCategories[0]?.value ?? 'overview';
			genCategoryLabel = genCategories[0]?.label ?? '';
		}
	}

	function clearGenSpace() {
		genSpaceSlug = '';
		genSpaceLabel = '';
		genCategories = [];
		genCategory = 'overview';
		genCategoryLabel = '';
	}

	async function genCategoryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.trim().toLowerCase();
		return genCategories
			.map((c) => ({ id: c.value, label: c.label }))
			.filter((i) => !needle || i.label.toLowerCase().includes(needle) || i.id.includes(needle));
	}

	function openGenerate() {
		genSpaceSlug = '';
		genSpaceLabel = '';
		genPageTitle = '';
		genPageSlug = '';
		genSlugTouched = false;
		genCategory = 'overview';
		genCategoryLabel = t('wiki.category_overview', 'Overview');
		genCategories = [];
		showGenerateModal = true;
		void loadGenCategories('');
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
				category: genCategory || 'overview'
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
			// row.status is the latest revision's status — gate on 'pending'
			// like the Revisions tab so reviewed rows don't offer dead actions.
			acts.push({
				label: t('app.wiki.approve_revision', 'Approve Revision'),
				icon: CheckCircle,
				onclick: approveRevision,
				show: (row: WikiItem) => row.status === 'pending' && !!row.latest_revision_id
			});
			acts.push({
				label: t('app.wiki.reject_revision', 'Reject Revision'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: WikiItem) => row.status === 'pending' && !!row.latest_revision_id
			});
		}
		if (canManage && canLock) {
			acts.push({
				label: t('app.wiki.toggle_lock', 'Toggle Lock'),
				icon: Lock,
				onclick: toggleLock,
				show: (row: WikiItem) => !row.deleted_at
			});
		}
		if (canManage) {
			acts.push({
				label: t('app.wiki.toggle_featured', 'Toggle Featured'),
				icon: Star,
				onclick: toggleFeatured,
				show: (row: WikiItem) => !row.deleted_at
			});
			acts.push({
				label: t('app.wiki.toggle_active', 'Toggle Active'),
				icon: CheckCircle,
				onclick: toggleActive,
				show: (row: WikiItem) => !row.deleted_at
			});
		}
		if (canDelete) {
			// Delete → built-in confirm → config.remove (soft). Force Delete →
			// page-local modal → hardDeletePage; only for soft-deleted rows,
			// and only for holders of admin.hard_delete (RequireForceDelete).
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: WikiItem, ctx) => ctx?.openDelete(row),
				show: (row: WikiItem) => !row.deleted_at
			});
		}
		if (canHardDelete) {
			acts.push({
				label: t('app.wiki.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openForceDelete,
				show: (row: WikiItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.wiki.col_title', 'Title'), type: 'text', help: t('app.wiki.title_hint', 'The title shown in listings and detail views.'),},
		{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), type: 'text', help: t('app.wiki.space_slug_hint', 'The space URL slug this belongs to.') },
		{ key: 'status', label: t('app.wiki.col_status', 'Status'), type: 'text', help: t('app.wiki.status_hint', 'The record state — controls visibility and actions.') },
		{ key: 'excerpt', label: t('app.wiki.detail_excerpt', 'Excerpt'), type: 'textarea', help: t('app.wiki.excerpt_hint', 'A short teaser shown in listings.') }
	];

	// ── Dropdown helpers — every dropdown field uses the shared Autocomplete
	// component; small static option sets get a client-side-filtered fetcher. ──
	function enumFetcher(items: AutocompleteItem[]) {
		return async (q: string): Promise<AutocompleteItem[]> => {
			const needle = q.trim().toLowerCase();
			return needle ? items.filter((i) => i.label.toLowerCase().includes(needle)) : items;
		};
	}
	const enumInitial = (items: AutocompleteItem[]) => async () => items;
	const enumLabel =
		(items: AutocompleteItem[]) => (id: string) => items.find((i) => i.id === id)?.label ?? id;

	const LOCKED_OPTIONS = $derived<AutocompleteItem[]>([
		{ id: 'true', label: t('app.wiki.filter_locked_yes', 'Locked') },
		{ id: 'false', label: t('app.wiki.filter_locked_no', 'Unlocked') }
	]);
	const FEATURED_OPTIONS = $derived<AutocompleteItem[]>([
		{ id: 'true', label: t('app.wiki.filter_featured_yes', 'Featured') },
		{ id: 'false', label: t('app.wiki.filter_featured_no', 'Not featured') }
	]);
	const ACTIVE_OPTIONS = $derived<AutocompleteItem[]>([
		{ id: 'true', label: t('app.wiki.filter_active_yes', 'Active') },
		{ id: 'false', label: t('app.wiki.filter_active_no', 'Inactive') }
	]);

	const KIND_OPTIONS = $derived<AutocompleteItem[]>([
		{ id: 'content', label: t('app.wiki.kind_content', 'Content') },
		{ id: 'fields', label: t('app.wiki.kind_fields', 'Fields') },
		{ id: 'mixed', label: t('app.wiki.kind_mixed', 'Mixed') }
	]);
	// Latest-revision status — shared by the pages ?status= filter (the
	// queue's "pages with pending work" workflow) and the revisions queue's
	// own ?status= filter; the backend whitelists the same three values.
	const STATUS_OPTIONS = $derived<AutocompleteItem[]>([
		{ id: 'pending', label: t('app.wiki.status_pending', 'Pending') },
		{ id: 'approved', label: t('app.wiki.status_approved', 'Approved') },
		{ id: 'rejected', label: t('app.wiki.status_rejected', 'Rejected') }
	]);

	// Category filter options — the union of every enabled wiki type's
	// category template (GET /wiki/categories without ?space=), cached once.
	let wikiCategories = $state<AutocompleteItem[]>([]);
	let catsLoaded = false;
	async function loadWikiCategories(): Promise<AutocompleteItem[]> {
		if (catsLoaded) return wikiCategories;
		const res = await apiRaw<{ data?: Array<{ category?: string; slug?: string; label?: string; label_key?: string }> }>(
			'/wiki/categories'
		);
		if (res.ok && Array.isArray(res.data?.data)) {
			wikiCategories = res.data.data
				.map((c) => {
					const token = (c.category ?? c.slug ?? '').trim();
					if (!token) return null;
					return { id: token, label: c.label_key ? t(c.label_key, c.label ?? token) : (c.label ?? token) };
				})
				.filter((c): c is AutocompleteItem => c !== null);
			catsLoaded = true;
		}
		return wikiCategories;
	}

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		void wikiCategories;
		return [
			{
				key: 'dest',
				label: t('app.wiki.col_destination', 'Destination'),
				type: 'autocomplete',
			help: t('app.wiki.dest_hint', 'The destination country this applies to.'),
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			{
				key: 'category',
				label: t('app.wiki.gen_category', 'Category'),
				type: 'autocomplete',
			help: t('app.wiki.category_hint', 'The category this belongs to — drives grouping and filters.'),
				placeholder: t('app.wiki.filter_category_ph', 'Search categories…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: async (q: string) => {
					const cats = await loadWikiCategories();
					const needle = q.trim().toLowerCase();
					return needle
						? cats.filter((c) => c.label.toLowerCase().includes(needle) || c.id.includes(needle))
						: cats;
				},
				initialFetcher: loadWikiCategories,
				getSelectedLabel: (id: string) => wikiCategories.find((c) => c.id === id)?.label ?? id
			},
			{
				key: 'locked',
				label: t('app.wiki.col_locked', 'Locked'),
				type: 'autocomplete',
			help: t('app.wiki.locked_hint', 'Locked records cannot be edited by regular admins.'),
				placeholder: t('app.wiki.filter_locked_ph', 'Lock state…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: enumFetcher(LOCKED_OPTIONS),
				initialFetcher: enumInitial(LOCKED_OPTIONS),
				getSelectedLabel: enumLabel(LOCKED_OPTIONS)
			},
			{
				key: 'featured',
				label: t('app.wiki.col_featured', 'Featured'),
				type: 'autocomplete',
			help: t('app.wiki.featured_hint', 'Featured records get priority placement.'),
				placeholder: t('app.wiki.col_featured', 'Featured'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: enumFetcher(FEATURED_OPTIONS),
				initialFetcher: enumInitial(FEATURED_OPTIONS),
				getSelectedLabel: enumLabel(FEATURED_OPTIONS)
			},
			{
				key: 'active',
				label: t('app.wiki.col_active', 'Active'),
				type: 'autocomplete',
			help: t('app.wiki.active_hint', 'Inactive records are hidden or disabled on the site.'),
				placeholder: t('app.wiki.col_active', 'Active'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: enumFetcher(ACTIVE_OPTIONS),
				initialFetcher: enumInitial(ACTIVE_OPTIONS),
				getSelectedLabel: enumLabel(ACTIVE_OPTIONS)
			},
			// Latest revision status — filters pages to pending/approved/
			// rejected (the review queue's primary workflow).
			{
				key: 'status',
				label: t('app.wiki.col_status', 'Status'),
				type: 'autocomplete',
			help: t('app.wiki.status_hint', 'The record state — controls visibility and actions.'),
				placeholder: t('app.wiki.col_status', 'Status'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: enumFetcher(STATUS_OPTIONS),
				initialFetcher: enumInitial(STATUS_OPTIONS),
				getSelectedLabel: enumLabel(STATUS_OPTIONS)
			},
			// Soft-deleted pages are hidden by default — this filter surfaces
			// them so the Force Delete action has targets (§80).
			{
				key: 'deleted',
				label: t('app.wiki.filter_deleted', 'Deleted'),
				type: 'select',
			help: t('app.wiki.deleted_hint', 'Marks the record deleted without removing the row.'),
				options: [
					{ value: 'only', label: t('app.wiki.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('app.wiki.filter_deleted_all', 'Include deleted') }
				]
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.wiki.review_queue_heading', 'Wiki Review Queue'),
		description: t('app.wiki.review_queue_desc', 'Review and approve pending wiki page revisions'),
		readonly: true,
		// One tab's table renders at a time, so both tabs can url-sync
		// (q/sort/dir/page/pageSize/filters); 'tab' is a RESERVED_PARAM and
		// tabPreserve=[] wipes the other tab's list state on switch.
		urlSync: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.wiki.col_title', 'Title'), sortable: true },
			{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), sortable: true },
			{
				key: 'status',
				label: t('app.wiki.col_status', 'Status'),
				sortable: true,
				render: (value: string, row: WikiItem) =>
					(row.deleted_at ? `${value || '—'} · ${t('app.wiki.filter_deleted', 'Deleted')}` : value || '—')
			},
			{
				key: 'ai_generated',
				label: t('app.wiki.col_ai', 'AI'),
				sortable: false,
				render: (value: boolean) => (value ? '🤖' : '—')
			},
			{
				key: 'is_featured',
				label: t('app.wiki.col_featured', 'Featured'),
				sortable: true,
				render: (value: boolean) => (value ? '★' : '—')
			},
			{
				key: 'is_locked',
				label: t('app.wiki.col_locked', 'Locked'),
				sortable: true,
				render: (value: boolean) => (value ? '🔒' : '—')
			},
			{
				key: 'is_active',
				label: t('app.wiki.col_active', 'Active'),
				sortable: true,
				render: (value: boolean) => ((value ?? true) ? '✓' : '—')
			},
			{
				key: 'created_at',
				label: t('app.wiki.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'updated_at',
				label: t('app.wiki.col_updated', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		bulkActions: pageBulkActions,
		deletePermission: 'wiki.delete',
		remove: async (id) => {
			const res = await wikiAdminApi.deletePage(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			// EntityPage's params.page is 0-based; the backend page param is 1-based.
			qs.set('page', String(params.page + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			if (params.filter?.category) qs.set('category', params.filter.category);
			if (params.filter?.locked) qs.set('locked', params.filter.locked);
			if (params.filter?.featured) qs.set('featured', params.filter.featured);
			if (params.filter?.active) qs.set('active', params.filter.active);
			if (params.filter?.status) qs.set('status', params.filter.status);
			if (params.filter?.deleted) qs.set('deleted', params.filter.deleted);
			const result = await apiRaw<{ data: WikiItem[]; pagination: { total: number } }>(
				`/admin/wiki${qs.toString() ? `?${qs.toString()}` : ''}`
			);
			if (result.ok) {
				const body = result.data as { data: WikiItem[]; pagination: { total: number } };
				const data = body.data ?? [];
				// Accumulate rather than clear — BulkBar's selectedIds persist
				// across page changes, so a cross-page selection must still
				// resolve its earlier rows. Fresh rows overwrite stale state
				// on every load.
				for (const row of data) pageRowMap.set(row.id, row);
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
	// KIND_OPTIONS/STATUS_OPTIONS are declared up by the pages filters.

	const revisionFilters = $derived.by<FilterDef[]>(() => [
		{
			key: 'kind',
			label: t('app.wiki.filter_kind', 'Kind'),
			type: 'autocomplete',
			help: t('app.wiki.kind_hint', 'The kind or class of this record.'),
			placeholder: t('app.wiki.filter_kind', 'Kind'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: enumFetcher(KIND_OPTIONS),
			initialFetcher: enumInitial(KIND_OPTIONS),
			getSelectedLabel: enumLabel(KIND_OPTIONS)
		},
		{
			key: 'status',
			label: t('app.wiki.col_status', 'Status'),
			type: 'autocomplete',
			help: t('app.wiki.status_hint', 'The record state — controls visibility and actions.'),
			placeholder: t('app.wiki.col_status', 'Status'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: enumFetcher(STATUS_OPTIONS),
			initialFetcher: enumInitial(STATUS_OPTIONS),
			getSelectedLabel: enumLabel(STATUS_OPTIONS)
		},
		{
			key: 'deleted',
			label: t('app.wiki.filter_deleted', 'Deleted'),
			type: 'select',
			help: t('app.wiki.deleted_hint', 'Marks the record deleted without removing the row.'),
			options: [
				{ value: 'only', label: t('app.wiki.filter_deleted_only', 'Deleted only') },
				{ value: 'all', label: t('app.wiki.filter_deleted_all', 'Include deleted') }
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
				show: (row: WikiRevisionItem) => row.status === 'pending' && !row.deleted_at
			});
			acts.push({
				label: t('app.wiki.reject_revision', 'Reject Revision'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openRevisionReject,
				show: (row: WikiRevisionItem) => row.status === 'pending' && !row.deleted_at
			});
		}
		if (canDelete) {
			// Delete → built-in confirm → revisionConfig.remove (soft).
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: WikiRevisionItem, ctx) => ctx?.openDelete(row),
				show: (row: WikiRevisionItem) => !row.deleted_at
			});
		}
		if (canHardDelete) {
			// Force Delete → page-local modal → hardDeleteWikiRevision; only
			// for soft-deleted rows and holders of admin.hard_delete
			// (backend RequireForceDelete on ?force=true).
			acts.push({
				label: t('app.wiki.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openRevisionForceDelete,
				show: (row: WikiRevisionItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	// Minimal FieldDef set — EntityPage requires `fields` even for readonly pages.
	const revisionFields: FieldDef[] = [
		{ key: 'summary', label: t('app.wiki.col_summary', 'Summary'), type: 'text', help: t('app.wiki.summary_hint', 'A one or two sentence overview shown on cards.') }
	];

	const revisionBulkActions = $derived.by<BulkAction[]>(() => {
		const acts: BulkAction[] = [];
		if (canReview) {
			acts.push(
				{
					label: t('app.wiki.bulk_approve', 'Approve Selected'),
					icon: CheckCircle,
					onclick: (ids) => bulkEach(pendingRevisions(ids), (r) => wikiAdminApi.approveWikiRevision(r.id)),
					disabled: (ids) => acting || pendingRevisions(ids).length === 0
				},
				{
					label: t('app.wiki.bulk_reject', 'Reject Selected'),
					icon: XCircle,
					variant: 'destructive',
					onclick: openBulkRejectRevisions,
					disabled: (ids) => acting || pendingRevisions(ids).length === 0
				}
			);
		}
		if (canHardDelete) {
			// Bulk soft delete is EntityPage's built-in "Delete Selected"
			// (config.remove); this adds the bulk hard-delete counterpart for
			// soft-deleted rows (admin.hard_delete, backend RequireForceDelete).
			acts.push({
				label: t('app.wiki.bulk_force_delete', 'Force Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openBulkForceDeleteRevisions,
				disabled: (ids) => acting || !revisionRowsFor(ids).some((r) => !!r.deleted_at)
			});
		}
		return acts;
	});

	const revisionConfig = $derived<EntityPageConfig>({
		title: t('app.wiki.revisions_heading', 'Revision Queue'),
		description: t('app.wiki.revisions_desc', 'Review pending wiki revisions, including community field ops'),
		readonly: true,
		// Same as the pages tab — one tab's table renders at a time so
		// url-synced list state can't collide ('tab' is a RESERVED_PARAM).
		urlSync: true,
		pageSize: 25,
		filters: revisionFilters,
		columns: [
			{ key: 'kind', label: t('app.wiki.col_kind', 'Kind'), sortable: true },
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
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.wiki.col_status', 'Status'),
				sortable: true,
				render: (value: string, row: WikiRevisionItem) =>
					row.deleted_at ? `${value} · ${t('app.wiki.filter_deleted', 'Deleted')}` : value
			},
			{
				key: 'created_at',
				label: t('app.wiki.col_created', 'Submitted'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: revisionActions,
		bulkActions: revisionBulkActions,
		deletePermission: 'wiki.delete',
		remove: async (id) => {
			const res = await wikiAdminApi.deleteWikiRevision(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		fields: revisionFields,
		load: async (params) => {
			const qs = new URLSearchParams();
			// EntityPage's params.page is 0-based; the backend page param is 1-based.
			qs.set('page', String(params.page + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			const kind = params.filter?.kind;
			if (kind && kind !== 'all') qs.set('kind', kind);
			const status = params.filter?.status;
			if (status && status !== 'all') qs.set('status', status);
			if (params.filter?.deleted) qs.set('deleted', params.filter.deleted);
			const result = await wikiAdminApi.listWikiRevisions(qs.toString());
			if (result.ok) {
				const data = result.data.data ?? [];
				for (const row of data) revisionRowMap.set(row.id, row);
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
	<title>{t('app.wiki.review_queue_page_title', 'Wiki Review Queue — {site} Admin')}</title>
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
	tabParam="tab"
	tabPreserve={[]}
/>

{#if view === 'pages'}
	{#if canManage}
		<div class="mb-4 flex justify-end">
			<Button onclick={openGenerate}>
				<Sparkles size={16} class="me-1.5" />
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
			{#if bulkReject}
				{t('app.wiki.bulk_reject_confirm', 'Are you sure you want to reject {count} selected revisions?').replace('{count}', String(bulkReject.rows.length))}
			{:else}
				{t('app.wiki.reject_confirm', 'Are you sure you want to reject the revision for "{title}"?').replace('{title}', rejectTargetLabel)}
			{/if}
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
		{#if pageError}
			<div
				class="rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{pageError}
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showRejectModal = false; bulkReject = null; }}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.wiki.rejecting', 'Rejecting...') : t('app.wiki.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- ── Force delete confirmation modal (wiki.delete) ─────────────────────── -->
<Modal bind:open={showForceModal} title={t('app.wiki.force_delete', 'Force Delete')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t('app.wiki.force_delete_confirm', 'Permanently delete "{title}"? The page and all its revisions will be removed. This cannot be undone.').replace('{title}', forceTargets[0]?.title ?? '')}
			{:else}
				{t('app.wiki.force_delete_confirm_bulk', 'Permanently delete {count} selected pages? The pages and all their revisions will be removed. This cannot be undone.').replace('{count}', String(forceTargets.length))}
			{/if}
		</p>
		{#if pageError}
			<div
				class="rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{pageError}
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTargets = []; }}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={acting}>
			{acting ? t('app.wiki.force_deleting', 'Deleting...') : t('app.wiki.force_delete', 'Force Delete')}
		</Button>
	</div>
</Modal>

<!-- ── Revision force delete (wiki.delete + admin.hard_delete) ──────────── -->
<Modal bind:open={showRevForceModal} title={t('app.wiki.force_delete', 'Force Delete')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if revForceTargets.length === 1}
				{t('app.wiki.force_delete_revision', 'Permanently delete revision "{title}"? This cannot be undone.').replace('{title}', revForceTargets[0]?.summary ?? revForceTargets[0]?.id ?? '')}
			{:else}
				{t('app.wiki.force_delete_revision_bulk', 'Permanently delete {count} selected revisions? This cannot be undone.').replace('{count}', String(revForceTargets.length))}
			{/if}
		</p>
		{#if pageError}
			<div
				class="rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{pageError}
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showRevForceModal = false; revForceTargets = []; }}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmRevisionForceDelete} disabled={acting}>
			{acting ? t('app.wiki.force_deleting', 'Deleting...') : t('app.wiki.force_delete', 'Force Delete')}
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
			<label for="gen-space-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.gen_space', 'Space')}</label>
			<Autocomplete
				id="gen-space-slug"
				placeholder={t('app.wiki.gen_space_ph', 'Search destination spaces…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={genSpaceFetcher}
				selectedId={genSpaceSlug || null}
				selectedLabel={genSpaceLabel}
				onSelect={selectGenSpace}
				onClear={clearGenSpace}
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
			<Autocomplete
				id="gen-category"
				placeholder={t('app.wiki.gen_category_ph', 'Search categories…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={genCategoryFetcher}
				initialFetcher={async () => genCategories.map((c) => ({ id: c.value, label: c.label }))}
				selectedId={genCategory || null}
				selectedLabel={genCategories.find((c) => c.value === genCategory)?.label ?? genCategoryLabel}
				onSelect={(item) => {
					genCategory = item.id;
					genCategoryLabel = item.label;
				}}
				onClear={() => {
					genCategory = '';
					genCategoryLabel = '';
				}}
			/>
		</div>
		{#if pageError}
			<div
				class="rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{pageError}
			</div>
		{/if}
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
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">
				{row.space_name ? `${row.space_name} (${row.space_slug})` : row.space_slug || '—'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.gen_page_slug', 'Page slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.gen_category', 'Category')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.category || '—'}</p>
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
		{#if row.field_group_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.cat_group', 'Field group')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					<a href="/custom-fields/{row.field_group_id}" class="underline" style="color: var(--color-primary)">
						{row.field_group_name || row.field_group_slug || row.field_group_id}
					</a>
					{#if row.field_group_slug}
						<span class="block font-mono text-xs" style="color: var(--color-muted)">{row.field_group_slug}</span>
					{/if}
				</p>
			</div>
		{/if}
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
		{#if row.deleted_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_deleted', 'Deleted')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-danger)">{new Date(row.deleted_at).toLocaleString()}</p>
			</div>
		{/if}
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
				<HintIcon text={t('app.wiki.data_entity_type_hint', 'Binds this page to a child entity class for community field ops. Empty clears the binding.')} />
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
		{#if row.deleted_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.filter_deleted', 'Deleted')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-danger)">{new Date(row.deleted_at).toLocaleString()}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_summary', 'Summary')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary || '—'}</p>
		</div>
		{#if row.wiki_page_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_page', 'Page')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.wiki_page_id}</p>
			</div>
		{/if}
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_ai', 'AI-generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{t('app.wiki.ai_yes', 'Yes')}{row.ai_model ? ` (${row.ai_model})` : ''}
				</p>
			</div>
		{/if}
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
		{#if row.hitl_review_notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.review_notes', 'Review notes')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.hitl_review_notes}</p>
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
