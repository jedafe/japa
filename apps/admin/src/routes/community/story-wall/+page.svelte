<script lang="ts">
	// /community/story-wall — admin HITL review queue for Story Wall submissions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { storyAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type StoryItem = {
		id: string;
		title: string;
		author_name: string;
		author_id: string;
		status: string;
		created_at: string;
		content?: string;
	};

	const canManage = $derived(hasPermission('story.verify'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<StoryItem | null>(null);
	let showDeleteModal = $state(false);

	function openDelete(item: StoryItem) {
		actionTarget = item;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.delete(actionTarget.id);
			if (result.ok) {
				showDeleteModal = false;
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

	async function verifyStory(item: StoryItem) {
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.verify(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: StoryItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.story.verify', 'Verify'),
				icon: CheckCircle,
				onclick: verifyStory,
				show: () => true
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.story.col_title', 'Title'), type: 'text' },
		{ key: 'author_name', label: t('app.story.col_author', 'Author'), type: 'text' },
		{ key: 'status', label: t('app.story.col_status', 'Status'), type: 'text' },
		{ key: 'content', label: t('app.story.detail_content', 'Content'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.story.review_queue_heading', 'Story Wall Review Queue'),
		description: t('app.story.review_queue_desc', 'Review and verify pending Story Wall submissions'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.story.col_title', 'Title'), sortable: true },
			{
				key: 'author_name',
				label: t('app.story.col_author', 'Author'),
				sortable: true,
				render: (value: string) => value || t('app.story.unknown', 'Unknown')
			},
			{
				key: 'status',
				label: t('app.story.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.story.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await storyAdminApi.list();
			if (result.ok) {
				const data = (result.data as StoryItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: StoryView
	});
</script>

<svelte:head>
	<title>{t('app.story.review_queue_page_title', 'Story Wall Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Delete confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showDeleteModal} title={t('app.story.modal_delete_story', 'Delete Story')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.story.delete_confirm', 'Are you sure you want to delete "{title}"? This cannot be undone.').replace('{title}', actionTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('app.story.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet StoryView({ row }: { row: StoryItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_author', 'Author')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.author_name || t('app.story.unknown', 'Unknown')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.detail_content', 'Content')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.content || '—'}</p>
		</div>
	</div>
{/snippet}
