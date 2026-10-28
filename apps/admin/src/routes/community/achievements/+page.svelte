<script lang="ts">
	// /community/achievements — admin moderation queue for achievements.
	// Lists all achievements (any status) with publish/unpublish/delete
	// actions gated by achievement.moderate. Uses server-side pagination via
	// the DataTable. Ref: §89, ADR-097.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { achievementAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, EyeOff, Trash2, CheckCircle } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type AchievementItem = {
		id: string;
		title: string;
		slug: string;
		body: string;
		status: string;
		milestone_type?: string;
		is_anonymous: boolean;
		author_name?: string;
		destination_space_name?: string;
		origin_space_name?: string;
		created_at: string;
		published_at?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('achievement.moderate'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);

	let achievementStatusOptions = $state<CatalogOption[]>([
		{ value: 'active', label: t('app.achievement.status_active', 'Active') },
		{ value: 'inactive', label: t('app.achievement.status_inactive', 'Inactive') },
		{ value: 'pending', label: t('app.achievement.status_pending', 'Pending') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('community.achievements.status');
			if (res.ok && res.data?.length) achievementStatusOptions = res.data;
		})();
	});

	let pageError = $state('');
	let actionLoading = $state(false);

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: AchievementItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.achievements.publish', 'Publish'),
				icon: CheckCircle,
				show: (row: AchievementItem) => row.status !== 'published',
				onclick: (row: AchievementItem) => setStatus(row, 'published')
			});
			acts.push({
				label: t('app.achievements.unpublish', 'Unpublish'),
				icon: EyeOff,
				show: (row: AchievementItem) => row.status === 'published',
				onclick: (row: AchievementItem) => setStatus(row, 'draft')
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: AchievementItem, ctx) => ctx?.openDelete(row)
			});
		}
		return acts;
	});

	async function setStatus(item: AchievementItem, status: string) {
		if (actionLoading) return;
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.achievement.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		actionLoading = true;
		pageError = '';
		try {
			const result = await achievementAdminApi.setStatus(item.id, status);
			if (!result.ok) throw new Error(result.error ?? t('common.error', 'An error occurred'));
			entityController?.reload();
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'An error occurred');
		} finally {
			actionLoading = false;
		}
	}

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.achievements.col_title', 'Title'), type: 'text', help: t('app.achievements.title_hint', 'The title shown in listings and detail views.'),},
		{ key: 'body', label: t('app.achievements.col_body', 'Body'), type: 'textarea', help: t('app.achievements.body_hint', 'The main content body.') },
		{ key: 'milestone_type', label: t('app.achievements.col_milestone', 'Milestone'), type: 'text', help: t('app.achievements.milestone_type_hint', 'The kind of milestone — e.g. visa approved.') },
		{ key: 'status', label: t('app.achievements.col_status', 'Status'), type: 'text', help: t('app.achievements.status_hint', 'The record state — controls visibility and actions.') },
		{ key: 'is_anonymous', label: t('app.achievements.col_anonymous', 'Anonymous'), type: 'checkbox', help: t('app.achievements.is_anonymous_hint', 'Hide the author’s identity publicly.') }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.achievement.col_status', 'Status'),
				options: achievementStatusOptions
			},
			{
				key: 'dest',
				label: t('app.achievement.col_destination', 'Destination'),
				type: 'autocomplete',
			help: t('app.achievement.dest_hint', 'The destination country this applies to.'),
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
		title: t('app.achievements.review_queue_heading', 'Achievements Moderation'),
		description: t('app.achievements.review_queue_desc', 'Review community achievement records'),
		// Achievements are user-submitted; admins moderate (publish/unpublish/
		// delete) via row actions, not create/edit. readonly suppresses the
		// Create toolbar button and the Edit row action (EntityPage gates both
		// on !config.readonly), neither of which has a backend handler here.
		readonly: true,
		deletePermission: 'achievement.moderate',
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.achievements.col_title', 'Title') },
			{
				key: 'author_name',
				label: t('app.achievements.col_user', 'User'),
				render: (value: string, row: AchievementItem) =>
					row.is_anonymous
						? t('app.achievements.anonymous', 'Anonymous')
						: (value || t('app.achievements.unknown', 'Unknown'))
			},
			{
				key: 'destination_space_name',
				label: t('app.achievements.col_destination', 'Destination'),
				render: (value: string) => value || '—'
			},
			{
				key: 'milestone_type',
				label: t('app.achievements.col_milestone', 'Milestone'),
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.achievements.col_status', 'Status'),
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.achievements.col_created', 'Created'),
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			qs.set('page', String((params.page ?? 0) + 1));
			qs.set('limit', String(params.pageSize ?? 25));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			if (params.filter?.status) qs.set('status', params.filter.status);
			const result = await achievementAdminApi.list(qs.toString());
			if (result.ok) {
				const data = (result.data.data as AchievementItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		remove: canManage
			? async (id: string) => {
					const result = await achievementAdminApi.delete(id);
					return { ok: result.ok, error: result.ok ? undefined : result.error };
				}
			: undefined,
		viewSnippet: AchievementView
	});
</script>

<svelte:head>
	<title>{t('app.achievements.review_queue_page_title', 'Achievements Moderation — {site} Admin')}</title>
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
{#snippet AchievementView({ row }: { row: AchievementItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_user', 'User')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_anonymous
					? t('app.achievements.anonymous', 'Anonymous')
					: (row.author_name || t('app.achievements.unknown', 'Unknown'))}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_destination', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		{#if row.origin_space_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_origin', 'Origin')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.origin_space_name}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_milestone', 'Milestone')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.milestone_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_body', 'Body')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.body || '—'}</p>
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
