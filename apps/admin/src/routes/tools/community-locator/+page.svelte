<script lang="ts">
	// /tools/community-locator — read-only HITL review queue with Approve + Verify actions.
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { communityLocatorAdminApi , spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, BadgeCheck, Archive, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

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

	// ── State ───────────────────────────────────────────────────────────────
	
// Destination Spaces for the dest filter (geo-aware pattern §90-101).
type SpaceOption = { id: string; name: string };
let destOptions = $state<SpaceOption[]>([]);
onMount(async () => {
	try {
		const res = await spacesAdminApi.list('?level=country&limit=200');
		if (res.ok && Array.isArray(res.data)) {
			destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
				id: s.id, name: s.name
			}));
		}
	} catch { /* Non-fatal */ }
});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	async function approveCommunity(item: CommunityItem) {
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

	async function archiveCommunity(item: CommunityItem) {
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.archive(item.id);
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

	async function deleteCommunity(item: CommunityItem) {
		acting = true;
		pageError = '';
		try {
			const result = await communityLocatorAdminApi.delete(item.id);
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
				show: (row: CommunityItem) => row.status === 'pending'
			});
			acts.push({
				label: t('common.verify', 'Verify'),
				icon: BadgeCheck,
				onclick: verifyCommunity,
				show: (row: CommunityItem) => !row.is_verified && row.status === 'published'
			});
			acts.push({
				label: t('common.archive', 'Archive'),
				icon: Archive,
				onclick: archiveCommunity,
				show: (row: CommunityItem) => row.status === 'published'
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				onclick: deleteCommunity,
				variant: 'destructive'
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'name', label: t('common.name', 'Name'), type: 'text' },
		{ key: 'org_type', label: t('common.type', 'Type'), type: 'text' },
		{ key: 'origin_community', label: t('common.origin', 'Origin'), type: 'text' },
		{ key: 'destination_space_name', label: t('common.country', 'Country'), type: 'text' },
		{ key: 'address', label: t('common.address', 'Address'), type: 'textarea' },
		{ key: 'website_url', label: t('common.website', 'Website'), type: 'url' },
		{ key: 'contact_info', label: t('common.contact', 'Contact'), type: 'text' },
		{ key: 'meeting_frequency', label: t('common.frequency', 'Meeting frequency'), type: 'text' }
	];

		const filters = $derived<FilterDef[]>([
		{ key: 'dest', label: t('app.tools.col_destination', 'Destination'), options: destOptions.map((s) => ({ value: s.id, label: s.name })) }
	]);

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.community_locator_heading', 'Community Locator Review Queue'),
		description: t('app.tools.community_locator_desc', 'Review, approve, and verify community locator entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('common.name', 'Name'), sortable: true },
			{ key: 'destination_space_name', label: t('common.country', 'Country'), sortable: true,
				render: (value: string | null) => value || '—' },
			{ key: 'org_type', label: t('common.type', 'Type'), sortable: true },
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
			qs.set('page', String(params.page ?? 1));
			qs.set('limit', String(params.pageSize ?? 25));
			if (params.q) qs.set('q', params.q);
			if (params.sort) {
				qs.set('sort', params.sort);
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
	</div>
{/snippet}
