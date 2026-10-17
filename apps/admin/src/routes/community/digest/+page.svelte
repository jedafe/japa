<script lang="ts">
	// /community/digest — admin HITL review queue for community digests (§84).
	// Lists digest editions with server-side pagination, filter, and sort.
	// Admin can publish drafts/reviews and trigger manual generation.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { digestAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles, Pencil } from 'lucide-svelte';
	import type { ActionContext, ActionItem, FilterDef } from '$lib/components/ui/data-table';

	// DigestItem matches the backend DigestEditionDTO (§84 API contract).
	// §84 is metrics-driven (no AI generation), so the digest_editions table
	// carries no ai_generated/ai_model/hitl_reviewed_by columns.
	type DigestItem = {
		id: string;
		space_id?: string;
		cadence: string;
		status: string;
		scheduled_for?: string;
		published_at?: string;
		email_sent: boolean;
		generated_by?: string;
		reviewed_by?: string;
		created_at: string;
	};

	const canManage = $derived(hasPermission('digest.manage'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);

	let digestStatusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: t('app.digest.status_draft', 'Draft') },
		{ value: 'review', label: t('app.digest.status_review', 'Review') },
		{ value: 'published', label: t('app.digest.status_published', 'Published') },
		{ value: 'cancelled', label: t('app.digest.status_cancelled', 'Cancelled') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('community.digest.status');
			if (res.ok && res.data?.length) digestStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);
	let showGenerateModal = $state(false);

	// ── View detail + edit (consume GET/PATCH /admin/digest/:id, §84) ─────────
	let viewDetail = $state<(DigestItem & { content?: Record<string, unknown> }) | null>(null);
	let viewDetailLoading = $state(false);
	let showEditModal = $state(false);
	let editTarget = $state<DigestItem | null>(null);
	let editJson = $state('');
	let editError = $state('');

	async function viewDigest(row: DigestItem, ctx?: ActionContext) {
		ctx?.openView(row);
		viewDetail = null;
		viewDetailLoading = true;
		try {
			const res = await digestAdminApi.get(row.id);
			if (res.ok) viewDetail = res.data as DigestItem & { content?: Record<string, unknown> };
		} finally {
			viewDetailLoading = false;
		}
	}

	// Content section labels per §84 (top_threads, new_members, events, story, poll).
	const sectionLabels = $derived<Record<string, string>>({
		top_threads: t('app.digest.section_top_threads', 'Top threads'),
		new_members: t('app.digest.section_new_members', 'New members'),
		events: t('app.digest.section_events', 'Events'),
		story: t('app.digest.section_story', 'Featured story'),
		poll: t('app.digest.section_poll', 'Poll')
	});

	function sectionSummary(value: unknown): string {
		if (Array.isArray(value)) return t('app.digest.section_items', '{count} items').replace('{count}', String(value.length));
		if (value && typeof value === 'object') {
			const v = value as Record<string, unknown>;
			return String(v.title ?? v.question ?? v.name ?? t('app.digest.section_set', 'Set'));
		}
		return '—';
	}

	async function openEditContent(row: DigestItem) {
		editError = '';
		acting = true;
		try {
			const res = await digestAdminApi.get(row.id);
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'An error occurred');
				return;
			}
			const detail = res.data as { content?: Record<string, unknown> } | undefined;
			editTarget = row;
			editJson = JSON.stringify(detail?.content ?? {}, null, 2);
			showEditModal = true;
		} finally {
			acting = false;
		}
	}

	async function saveEditContent() {
		if (!editTarget) return;
		let parsed: unknown;
		try {
			parsed = JSON.parse(editJson);
		} catch {
			editError = t('app.digest.invalid_json', 'Content must be valid JSON');
			return;
		}
		if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
			editError = t('app.digest.invalid_content', 'Content must be a JSON object');
			return;
		}
		acting = true;
		editError = '';
		try {
			const res = await digestAdminApi.update(editTarget.id, { content: parsed });
			if (res.ok) {
				showEditModal = false;
				entityController?.reload();
			} else {
				editError = res.error ?? t('common.error', 'An error occurred');
			}
		} finally {
			acting = false;
		}
	}

	async function publishDigest(item: DigestItem) {
		acting = true;
		pageError = '';
		try {
			const result = await digestAdminApi.publish(item.id);
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

	function openGenerate() {
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		acting = true;
		pageError = '';
		try {
			const result = await digestAdminApi.generate();
			if (result.ok) {
				showGenerateModal = false;
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: viewDigest }
		];
		if (canManage) {
			acts.push({
				label: t('app.digest.edit_content', 'Edit Content'),
				icon: Pencil,
				onclick: (row: DigestItem) => openEditContent(row),
				show: (row: DigestItem) => row.status === 'draft' || row.status === 'review'
			});
			acts.push({
				label: t('app.digest.publish', 'Publish'),
				icon: CheckCircle,
				onclick: publishDigest,
				show: (row: DigestItem) => row.status !== 'published' && row.status !== 'cancelled'
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'cadence', label: t('app.digest.col_cadence', 'Cadence'), type: 'text' },
		{ key: 'status', label: t('app.digest.col_status', 'Status'), type: 'text' }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.digest.col_status', 'Status'),
				options: digestStatusOptions
			},
			{
				key: 'dest',
				label: t('app.digest.col_destination', 'Destination'),
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
		title: t('app.digest.review_queue_heading', 'Digest Review Queue'),
		description: t('app.digest.review_queue_desc', 'Review and publish community digests'),
		readonly: true,
		pageSize: 25,
		filters,
		columns: [
			{ key: 'cadence', label: t('app.digest.col_cadence', 'Cadence'), sortable: true },
			{ key: 'status', label: t('app.digest.col_status', 'Status'), sortable: true },
			{
				key: 'scheduled_for',
				label: t('app.digest.col_scheduled_for', 'Scheduled For'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'published_at',
				label: t('app.digest.col_published_at', 'Published'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'email_sent',
				label: t('app.digest.col_email_sent', 'Email Sent'),
				render: (value: boolean) => (value ? '✓' : '—')
			},
			{
				key: 'created_at',
				label: t('app.digest.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// EntityPage passes a 0-based page; the backend expects 1-based.
			const queryParts: string[] = [`page=${params.page + 1}`, `limit=${params.pageSize}`];
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				// EntityPage passes sort as "key:dir" (e.g. "created_at:desc").
				const [sortKey, sortDir] = params.sort.split(':');
				if (sortKey) queryParts.push(`sort=${encodeURIComponent(sortKey)}`);
				if (sortDir) queryParts.push(`order=${sortDir === 'asc' ? 'asc' : 'desc'}`);
			}
			if (params.filter?.status) queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			const queryString = queryParts.join('&');

			const result = await digestAdminApi.list(queryString);
			if (result.ok) {
				const data = (result.data.data as DigestItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: DigestView
	});
</script>

<svelte:head>
	<title>{t('app.digest.review_queue_page_title', 'Digest Review Queue — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	{#if canManage}
		<Button onclick={openGenerate} disabled={acting}>
			<Sparkles class="mr-1.5 h-4 w-4" />
			{t('app.digest.generate', 'Generate')}
		</Button>
	{/if}
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Generate digest modal ──────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.digest.modal_generate', 'Generate Digest')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.digest.generate_desc', 'Generate a new community digest draft from recent activity. The edition enters a 1-hour review window before auto-publish.')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('app.digest.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting}>
			{acting ? t('app.digest.generating', 'Generating...') : t('app.digest.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── Edit content slide-over (draft/review only — PATCH /admin/digest/:id) ── -->
<SlideOver bind:open={showEditModal} title={t('app.digest.modal_edit', 'Edit Digest Content')} size="lg">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-muted)">
			{t('app.digest.edit_desc', 'Edit the digest content JSON (sections: top_threads, new_members, events, story, poll).')}
		</p>
		{#if editError}
			<p class="text-sm" style="color: var(--color-danger)">{editError}</p>
		{/if}
		<textarea
			bind:value={editJson}
			rows="20"
			class="w-full rounded-md border p-2 font-mono text-xs"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
		></textarea>
	</div>
	{#snippet footer()}
		<Button variant="ghost" onclick={() => (showEditModal = false)}>{t('app.digest.cancel', 'Cancel')}</Button>
		<Button onclick={saveEditContent} disabled={acting}>
			{acting ? t('app.digest.saving', 'Saving...') : t('app.digest.save', 'Save')}
		</Button>
	{/snippet}
</SlideOver>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet DigestView({ row }: { row: DigestItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_cadence', 'Cadence')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.cadence || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_scheduled_for', 'Scheduled For')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.scheduled_for ? new Date(row.scheduled_for).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_published_at', 'Published')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.published_at ? new Date(row.published_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_email_sent', 'Email Sent')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.email_sent ? '✓' : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_generated_by', 'Generated By')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.generated_by || '—'}</p>
		</div>
		{#if row.reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_reviewed_by', 'Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.reviewed_by}</p>
			</div>
		{/if}
		{#if row.space_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_space_id', 'Space')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.space_id}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_content', 'Content')}</p>
			{#if viewDetailLoading}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
			{:else if viewDetail?.content && Object.keys(viewDetail.content).length}
				<ul class="mt-1 space-y-1">
					{#each Object.entries(viewDetail.content) as [key, value] (key)}
						<li class="text-sm" style="color: var(--color-foreground)">
							<span class="font-medium">{sectionLabels[key] ?? key}</span>
							<span style="color: var(--color-muted)"> — {sectionSummary(value)}</span>
						</li>
					{/each}
				</ul>
			{:else if viewDetail}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('app.digest.no_content', 'No content sections')}</p>
			{/if}
		</div>
	</div>
{/snippet}
