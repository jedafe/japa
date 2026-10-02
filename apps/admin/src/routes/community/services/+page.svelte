<script lang="ts">
	// /community/services — admin HITL review queue for service directory listings.
	// Ref: japaconnect-engineering/sections/98-service-directory.md (ADR-106)
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { serviceDirectoryAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, BadgeCheck, XCircle } from 'lucide-svelte';
	import { Modal } from '$lib/components/ui/modal';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	// Backend ServiceRow shape (§98 DTO). The admin list endpoint returns
	// { data: ServiceRow[], meta: { page, limit, total } }.
	type ServiceItem = {
		id: string;
		title: string;
		slug: string;
		service_category?: string;
		status: string;
		is_verified: boolean;
		claim_status?: string;
		claimed_by_org_id?: string;
		claimed_org_name?: string;
		created_at: string;
		body?: string;
		author_name?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('service.verify'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);

	let serviceStatusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: t('app.services.status_draft', 'Draft') },
		{ value: 'published', label: t('app.services.status_published', 'Published') },
		{ value: 'pending', label: t('app.services.status_pending', 'Pending') },
		{ value: 'archived', label: t('app.services.status_archived', 'Archived') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('community.services.status');
			if (res.ok && res.data?.length) serviceStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let acting = $state(false);

	// Resolve-claim modal state (BUG-012).
	let claimTarget = $state<ServiceItem | null>(null);

	async function verifyService(item: ServiceItem) {
		// ADR-088 HITL gate: AI-generated content cannot be verified without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.services.hitl_required', 'Cannot verify AI-generated service without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await serviceDirectoryAdminApi.verify(item.id);
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

	async function unverifyService(item: ServiceItem) {
		acting = true;
		pageError = '';
		try {
			const result = await serviceDirectoryAdminApi.unverify(item.id);
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

	async function resolveClaim(item: ServiceItem, approved: boolean) {
		if (!item) return;
		acting = true;
		pageError = '';
		try {
			// The :claim_id URL param is the org_id of the claimant (§98).
			// The backend matches the claim by content_id + claimed_by_org_id.
			const claimId = item.claimed_by_org_id ?? item.id;
			const result = await serviceDirectoryAdminApi.resolveClaim(item.id, claimId, {
				approve: approved
			});
			if (result.ok) {
				claimTarget = null;
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ServiceItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.services.verify', 'Verify'),
				icon: CheckCircle,
				onclick: verifyService,
				show: (row: ServiceItem) => !row.is_verified
			});
			acts.push({
				label: t('app.services.unverify', 'Unverify'),
				icon: XCircle,
				onclick: unverifyService,
				show: (row: ServiceItem) => row.is_verified
			});
			acts.push({
				label: t('app.services.resolve_claim', 'Resolve claim'),
				icon: BadgeCheck,
				onclick: (row: ServiceItem) => (claimTarget = row),
				show: (row: ServiceItem) => row.claim_status === 'pending'
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.services.col_name', 'Title'), type: 'text' },
		{ key: 'service_category', label: t('app.services.col_category', 'Category'), type: 'text' },
		{ key: 'status', label: t('app.services.col_status', 'Status'), type: 'text' },
		{ key: 'body', label: t('app.services.detail_description', 'Description'), type: 'textarea' }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.services.col_status', 'Status'),
				options: serviceStatusOptions
			},
			{
				key: 'dest',
				label: t('app.services.col_destination', 'Destination'),
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
		title: t('app.services.review_queue_heading', 'Service Directory Review Queue'),
		description: t('app.services.review_queue_desc', 'Verify community service directory listings'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.services.col_name', 'Title'), sortable: true },
			{ key: 'service_category', label: t('app.services.col_category', 'Category'), sortable: true },
			{ key: 'status', label: t('app.services.col_status', 'Status'), sortable: true },
			{
				key: 'is_verified',
				label: t('app.services.col_verified', 'Verified'),
				sortable: true,
				render: (value: boolean) => (value ? t('app.services.yes', 'Yes') : t('app.services.no', 'No'))
			},
			{
				key: 'claim_status',
				label: t('app.services.col_claim', 'Claim'),
				sortable: true,
				render: (value: string) => value ?? '—'
			},
			{
				key: 'created_at',
				label: t('app.services.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			qs.set('page', String(params.page + 1));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			if (params.filter?.status) qs.set('status', params.filter.status);
			const result = await serviceDirectoryAdminApi.list(qs.toString());
			if (result.ok && result.data) {
				const envelope = result.data;
				const data = (envelope.data as ServiceItem[]) ?? [];
				const total = envelope.meta?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize || 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.ok ? '' : result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ServiceView
	});
</script>

<svelte:head>
	<title>{t('app.services.review_queue_page_title', 'Service Directory Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Resolve-claim confirm modal (BUG-012) ─────────────────────────────── -->
<Modal
	open={claimTarget !== null}
	onclose={() => (claimTarget = null)}
	title={t('app.services.resolve_claim_title', 'Resolve organization claim')}
>
	{#if claimTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.services.resolve_claim_prompt', 'An organization is claiming this listing. Approve or reject the claim.')}
		</p>
		{#if claimTarget.claimed_org_name}
			<p class="mt-2 text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.services.claiming_org', 'Claiming organization')}: {claimTarget.claimed_org_name}
			</p>
		{/if}
		<div class="mt-4 flex justify-end gap-2">
			<button
				type="button"
				onclick={() => resolveClaim(claimTarget!, false)}
				disabled={acting}
				class="inline-flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
				style="background: color-mix(in srgb, var(--color-danger) 15%, transparent); color: var(--color-danger)"
			>
				<XCircle size={16} />
				{t('app.services.reject_claim', 'Reject')}
			</button>
			<button
				type="button"
				onclick={() => resolveClaim(claimTarget!, true)}
				disabled={acting}
				class="inline-flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background: var(--color-success)"
			>
				<CheckCircle size={16} />
				{t('app.services.approve_claim', 'Approve')}
			</button>
		</div>
	{/if}
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ServiceView({ row }: { row: ServiceItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_name', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.service_category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_verified', 'Verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_verified ? t('app.services.yes', 'Yes') : t('app.services.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_claim', 'Claim status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.claim_status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.detail_description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.body || '—'}</p>
		</div>
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
	</div>
{/snippet}
