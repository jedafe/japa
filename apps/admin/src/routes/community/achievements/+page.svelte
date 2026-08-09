<script lang="ts">
	// /community/achievements — read-only admin moderation list for achievements.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { achievementAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type AchievementItem = {
		id: string;
		title: string;
		user_name: string;
		space_slug: string;
		created_at: string;
		description?: string;
	};

	const canManage = $derived(hasPermission('achievement.moderate'));

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── Row actions (View only — backend is a list endpoint) ─────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: AchievementItem, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.achievements.col_title', 'Title'), type: 'text' },
		{ key: 'user_name', label: t('app.achievements.col_user', 'User'), type: 'text' },
		{ key: 'space_slug', label: t('app.achievements.col_space', 'Space'), type: 'text' },
		{ key: 'description', label: t('app.achievements.detail_description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.achievements.review_queue_heading', 'Achievements Moderation'),
		description: t('app.achievements.review_queue_desc', 'Review community achievement records'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.achievements.col_title', 'Title'), sortable: true },
			{
				key: 'user_name',
				label: t('app.achievements.col_user', 'User'),
				sortable: true,
				render: (value: string) => value || t('app.achievements.unknown', 'Unknown')
			},
			{ key: 'space_slug', label: t('app.achievements.col_space', 'Space'), sortable: true },
			{
				key: 'created_at',
				label: t('app.achievements.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await achievementAdminApi.list();
			if (result.ok) {
				const data = (result.data as AchievementItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: AchievementView
	});
</script>

<svelte:head>
	<title>{t('app.achievements.review_queue_page_title', 'Achievements Moderation — JapaConnect Admin')}</title>
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
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.user_name || t('app.achievements.unknown', 'Unknown')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_space', 'Space')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.space_slug || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.achievements.detail_description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
