<script lang="ts">
	// /tools/community-locator — read-only HITL review queue with Approve + Verify actions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { communityLocatorAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, BadgeCheck } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type CommunityItem = {
		id: string;
		name: string;
		dest_country: string;
		type: string;
		status: string;
		verified: boolean;
		created_at: string;
		description?: string;
		location?: string;
	};

	const canManage = $derived(hasPermission('community_locator.manage'));

	// ── State ───────────────────────────────────────────────────────────────
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
				show: () => true
			});
			acts.push({
				label: t('common.verify', 'Verify'),
				icon: BadgeCheck,
				onclick: verifyCommunity,
				show: (row: CommunityItem) => !row.verified
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'name', label: t('common.name', 'Name'), type: 'text' },
		{ key: 'dest_country', label: t('common.country', 'Country'), type: 'text' },
		{ key: 'type', label: t('common.type', 'Type'), type: 'text' },
		{ key: 'description', label: t('common.description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.community_locator_heading', 'Community Locator Review Queue'),
		description: t('app.tools.community_locator_desc', 'Review, approve, and verify community locator entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('common.name', 'Name'), sortable: true },
			{ key: 'dest_country', label: t('common.country', 'Country'), sortable: true },
			{ key: 'type', label: t('common.type', 'Type'), sortable: true },
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'verified',
				label: t('common.verified', 'Verified'),
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
		load: async () => {
			const result = await communityLocatorAdminApi.list();
			if (result.ok) {
				const data = (result.data as CommunityItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
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
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.verified', 'Verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.verified ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.location}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.location', 'Location')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.location}</p>
			</div>
		{/if}
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.description', 'Description')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
	</div>
{/snippet}
