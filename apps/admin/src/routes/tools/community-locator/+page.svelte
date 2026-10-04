<script lang="ts">
	// /tools/community-locator — HITL review queue with Approve, Verify,
	// claim-resolution, archive/delete, and AI draft generation actions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { communityLocatorAdminApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, BadgeCheck, Archive, Trash2, Hand, XCircle } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';

	// CommunityItem matches the OrgDTO field names returned by the backend
	// (internal/communitylocator/service.go OrgDTO). The previous version used
	// mismatched field names (dest_country, type, verified) that didn't match
	// the API contract, causing empty table cells.
	type CommunityItem = {
		id: string;
		name: string;
		slug: string;
		org_type: string;
		origin_community: string;
		destination_space_name?: string | null;
		city_space_name?: string | null;
		claim_status: string;
		is_verified: boolean;
		is_active: boolean;
		status: string;
		ai_generated: boolean;
		ai_model?: string | null;
		hitl_reviewed_by?: string | null;
		ai_generated_at?: string | null;
		is_community_submitted: boolean;
		submitter_name?: string | null;
		created_at: string;
		updated_at: string;
		address?: string | null;
		website_url?: string | null;
		meeting_frequency?: string | null;
		contact_info?: string | null;
	};

	const canManage = $derived(hasPermission('community_locator.manage'));

	function prettifyEnum(value: string): string {
		return value.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// Org-type allow-list from the backend (Rule 16) — drives the type
	// filter dropdown and the generate-draft form's org_type select.
	let orgTypeOptions = $state<CatalogOption[]>([]);
	$effect(() => {
		communityLocatorAdminApi.orgTypes().then((res) => {
			if (res.ok && Array.isArray(res.data)) orgTypeOptions = res.data;
		});
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Destructive-action confirmation (archive / delete).
	type PendingAction = { kind: 'archive' | 'delete'; item: CommunityItem } | null;
	let pendingAction = $state<PendingAction>(null);
	let confirmOpen = $state(false);

	function requestAction(kind: 'archive' | 'delete', item: CommunityItem) {
		pendingAction = { kind, item };
		confirmOpen = true;
	}

	async function confirmAction() {
		if (!pendingAction || acting) return;
		const { kind, item } = pendingAction;
		acting = true;
		pageError = '';
		try {
			const result =
				kind === 'archive'
					? await communityLocatorAdminApi.archive(item.id)
					: await communityLocatorAdminApi.delete(item.id);
			if (result.ok) {
				confirmOpen = false;
				pendingAction = null;
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

	async function approveCommunity(item: CommunityItem) {
		if (acting) return;
		// ADR-088: the ApproveOrg backend call IS the HITL review gate — for
		// ai_generated rows it stamps hitl_reviewed_by/hitl_reviewed_at. A
		// client-side gate here would leave ai_draft rows unapprovable.
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.approve(item.id);
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

	async function verifyCommunity(item: CommunityItem) {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.verify(item.id);
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

	// BUG-004: wire the claim-resolution actions so pending claims can be
	// approved/rejected from the admin review queue. Claims are 1:1 with an
	// org — the backend enforces :claim_id === :id, so we pass the org id as
	// both path params. Body: { approve: bool }.
	async function approveClaim(item: CommunityItem) {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.resolveClaim(item.id, item.id, { approve: true });
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

	async function rejectClaim(item: CommunityItem) {
		if (acting) return;
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.resolveClaim(item.id, item.id, { approve: false });
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CommunityItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveCommunity,
				show: (row: CommunityItem) => row.status === 'pending' || row.status === 'ai_draft'
			});
			acts.push({
				label: t('common.verify', 'Verify'),
				icon: BadgeCheck,
				onclick: verifyCommunity,
				show: (row: CommunityItem) => !row.is_verified && row.status === 'published'
			});
			// BUG-004: claim-resolution actions. Shown only when the org has
			// a pending claim awaiting admin review.
			acts.push({
				label: t('app.community_locator.approve_claim', 'Approve claim'),
				icon: Hand,
				onclick: approveClaim,
				show: (row: CommunityItem) => row.claim_status === 'pending'
			});
			acts.push({
				label: t('app.community_locator.reject_claim', 'Reject claim'),
				icon: XCircle,
				onclick: rejectClaim,
				show: (row: CommunityItem) => row.claim_status === 'pending',
				variant: 'destructive'
			});
			acts.push({
				label: t('common.archive', 'Archive'),
				icon: Archive,
				onclick: (row: CommunityItem) => requestAction('archive', row),
				show: (row: CommunityItem) => row.status === 'published'
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				onclick: (row: CommunityItem) => requestAction('delete', row),
				variant: 'destructive'
			});
		}
		return acts;
	});

	// Fields drive the "Generate draft" create form (POST
	// /admin/community-locator/generate, ADR-088). Field keys map 1:1 to the
	// backend GenerateInput contract.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtils.ready;
		return [
			{ key: 'name', label: t('common.name', 'Name'), type: 'text', required: true },
			{
				key: 'destination_space_id',
				label: t('common.country', 'Country'),
				type: 'autocomplete',
				required: true,
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			{
				key: 'org_type',
				label: t('common.type', 'Type'),
				type: 'select',
				options: orgTypeOptions.map((o) => ({ value: o.value, label: t(`app.community_locator.type_${o.value}`, o.label) }))
			},
			{ key: 'origin_community', label: t('common.origin', 'Origin community'), type: 'text' },
			{ key: 'prompt', label: t('app.community_locator.prompt', 'AI prompt'), type: 'textarea', help: t('app.community_locator.prompt_help', 'Optional free-text prompt sent to the content agent.') },
			{ key: 'address', label: t('common.address', 'Address'), type: 'textarea' },
			{ key: 'website_url', label: t('common.website', 'Website'), type: 'url' },
			{ key: 'contact_info', label: t('common.contact', 'Contact'), type: 'text' },
			{ key: 'meeting_frequency', label: t('common.frequency', 'Meeting frequency'), type: 'text' }
		];
	});

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		orgTypeOptions;
		return [
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			{
				key: 'status',
				label: t('common.status', 'Status'),
				type: 'select',
				// Lifecycle enum from the backend store (internal/communitylocator).
				options: ['pending', 'ai_draft', 'published', 'archived', 'draft'].map((v) => ({
					value: v,
					label: t(`app.community_locator.status_${v}`, v)
				}))
			},
			{
				key: 'type',
				label: t('common.type', 'Type'),
				type: 'select',
				options: orgTypeOptions.map((o) => ({ value: o.value, label: t(`app.community_locator.type_${o.value}`, o.label) }))
			},
			{
				key: 'origin',
				label: t('common.origin', 'Origin'),
				type: 'text',
				placeholder: t('app.community_locator.origin_placeholder', 'e.g. Nigerian')
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.community_locator_heading', 'Community Locator Review Queue'),
		description: t('app.tools.community_locator_desc', 'Review, approve, and verify community locator entries'),
		// Not readonly: the page exposes the ADR-088 "Generate draft" create
		// flow (POST /admin/community-locator/generate). Row mutations are
		// handled by the custom actions above; no edit/delete defaults apply.
		createLabel: t('app.community_locator.generate_draft', 'Generate draft'),
		createPermission: 'community_locator.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('common.name', 'Name'), sortable: true },
			{ key: 'destination_space_name', label: t('common.country', 'Country'), sortable: true,
				render: (value: string | null) => value || '—' },
			{
				key: 'org_type',
				label: t('common.type', 'Type'),
				sortable: true,
				render: (value: string) => (value ? t(`app.community_locator.type_${value}`, prettifyEnum(value)) : '—')
			},
			{ key: 'origin_community', label: t('common.origin', 'Origin'), sortable: true,
				render: (value: string | null) => value || '—' },
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'claim_status',
				label: t('common.claim_status', 'Claim'),
				render: (value: string) => value || '—'
			},
			{
				key: 'is_verified',
				label: t('common.verified', 'Verified'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'ai_generated',
				label: t('common.ai_generated', 'AI'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Build query string with server-side pagination, search, and sort.
			const qs = new URLSearchParams();
			qs.set('page', String((params.page ?? 0) + 1));
			qs.set('limit', String(params.pageSize ?? 25));
			if (params.q) qs.set('q', params.q);
			if (params.sort) {
				// EntityPage emits the composite "key:dir" form; the backend
				// contract is separate ?sort=&dir= params (api-routes-catalog).
				const [sortKey, sortDir] = params.sort.split(':');
				qs.set('sort', sortKey);
				if (sortDir) qs.set('dir', sortDir);
			}
			if (params.filter) {
				for (const [k, v] of Object.entries(params.filter)) {
					if (v != null && v !== '') qs.set(k, String(v));
				}
			}
			const result = await communityLocatorAdminApi.list(qs.toString());
			if (result.ok) {
				const data = (result.data.data as CommunityItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		// ADR-088 AI draft generation — POST /admin/community-locator/generate.
		// Creates an ai_draft row that lands back in this queue for HITL review.
		create: async (body: Record<string, unknown>) => {
			const res = await communityLocatorAdminApi.generate({
				name: body.name,
				destination_space_id: body.destination_space_id,
				org_type: body.org_type || undefined,
				origin_community: body.origin_community || undefined,
				prompt: body.prompt || undefined,
				address: body.address || undefined,
				website_url: body.website_url || undefined,
				contact_info: body.contact_info || undefined,
				meeting_frequency: body.meeting_frequency || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CommunityView
	});
</script>

<svelte:head>
	<title>{t('app.tools.community_locator_page_title', 'Community Locator Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Destructive-action confirmation (archive / delete) ────────────────── -->
<Modal
	bind:open={confirmOpen}
	title={pendingAction?.kind === 'archive'
		? t('app.community_locator.confirm_archive_title', 'Archive organisation')
		: t('app.community_locator.confirm_delete_title', 'Delete organisation')}
>
	{#if pendingAction}
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if pendingAction.kind === 'archive'}
				{t('app.community_locator.confirm_archive_body', 'Archive')}
				<strong>{pendingAction.item.name}</strong>?
				{t('app.community_locator.confirm_archive_warn', 'It will be hidden from the public locator.')}
			{:else}
				{t('app.community_locator.confirm_delete_body', 'Delete')}
				<strong>{pendingAction.item.name}</strong>?
				{t('app.community_locator.confirm_delete_warn', 'This cannot be undone from this page.')}
			{/if}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" size="sm" onclick={() => (confirmOpen = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button
			size="sm"
			onclick={confirmAction}
			disabled={acting}
			class="bg-[var(--color-danger)] text-white hover:opacity-90"
		>
			{pendingAction?.kind === 'archive' ? t('common.archive', 'Archive') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet CommunityView({ row }: { row: CommunityItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.country', 'Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.org_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.origin', 'Origin community')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.origin_community || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.claim_status', 'Claim status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.claim_status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.verified', 'Verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_verified ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.ai_generated', 'AI generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.address}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.address', 'Address')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.address}</p>
			</div>
		{/if}
		{#if row.website_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.website', 'Website')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.website_url}</p>
			</div>
		{/if}
		{#if row.meeting_frequency}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.frequency', 'Meeting frequency')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.meeting_frequency}</p>
			</div>
		{/if}
		{#if row.contact_info}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.contact', 'Contact info')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.contact_info}</p>
			</div>
		{/if}
		{#if row.submitter_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.submitted_by', 'Submitted by')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.submitter_name}</p>
			</div>
		{/if}
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
