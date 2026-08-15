<script lang="ts">
	// /community/wiki — admin HITL review queue for wiki page revisions.
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { wikiAdminApi, apiRaw , spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Sparkles, Lock, Unlock, Star } from 'lucide-svelte';
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
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canReview = $derived(hasPermission('wiki.review'));
	const canManage = $derived(hasPermission('wiki.manage'));
	const canLock = $derived(hasPermission('wiki.lock'));

	// ── State-action confirmation modal (reject with optional reason) ────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<WikiItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

	function openReject(item: WikiItem) {
		actionTarget = item;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!actionTarget || !actionTarget.latest_revision_id) return;
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.rejectRevision(actionTarget.id, actionTarget.latest_revision_id, { reason: rejectReason });
			if (result.ok) {
				showRejectModal = false;
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

	async function approveRevision(item: WikiItem) {
		if (!item.latest_revision_id) return;
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

	// ── AI generate draft modal (wiki.manage) ───────────────────────────────
	let showGenerateModal = $state(false);
	let genSpaceSlug = $state('');
	let genPageTitle = $state('');
	let genCategory = $state('overview');
	let generating = $state(false);

	function openGenerate() {
		genSpaceSlug = '';
		genPageTitle = '';
		genCategory = 'overview';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!genSpaceSlug.trim() || !genPageTitle.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.generate({
				space_slug: genSpaceSlug.trim(),
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
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.wiki.col_title', 'Title'), type: 'text' },
		{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), type: 'text' },
		{ key: 'status', label: t('app.wiki.col_status', 'Status'), type: 'text' },
		{ key: 'excerpt', label: t('app.wiki.detail_excerpt', 'Excerpt'), type: 'textarea' }
	];

	
// Destination Spaces for the dest filter (geo-aware pattern). Loaded once
// on mount from /admin/spaces?level=country so the filter options are
// driven by the backend (Rule 16 — no hardcoded lists).
type SpaceOption = { id: string; name: string };
let destOptions = $state<SpaceOption[]>([]);
onMount(async () => {
	try {
		const res = await spacesAdminApi.list('?level=country&limit=200');
		if (res.ok && Array.isArray(res.data)) {
			destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
				id: s.id,
				name: s.name
			}));
		}
	} catch {
		// Non-fatal — the dest filter simply stays empty.
	}
});

		const filters = $derived<FilterDef[]>([
		{
			key: 'dest',
			label: t('app.wiki.col_destination', 'Destination'),
			options: destOptions.map((s) => ({ value: s.id, label: s.name }))
		}
	]);

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
			if (params.page) qs.set('page', String(params.page));
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

{#if canManage}
	<div class="mb-4 flex justify-end">
		<Button onclick={openGenerate}>
			<Sparkles size={16} class="mr-1.5" />
			{t('app.wiki.generate_draft', 'Generate AI Draft')}
		</Button>
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Reject revision confirmation modal ─────────────────────────────────── -->
<Modal bind:open={showRejectModal} title={t('app.wiki.modal_reject_revision', 'Reject Revision')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.wiki.reject_confirm', 'Are you sure you want to reject the revision for "{title}"?').replace('{title}', actionTarget?.title ?? '')}
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
				bind:value={genPageTitle}
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
				<option value="overview">Overview</option>
				<option value="visa">Visa</option>
				<option value="cost_of_living">Cost of Living</option>
				<option value="healthcare">Healthcare</option>
				<option value="banking">Banking</option>
				<option value="community">Community</option>
				<option value="housing">Housing</option>
				<option value="other">Other</option>
			</select>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !genSpaceSlug.trim() || !genPageTitle.trim()}>
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
	</div>
{/snippet}
