<script lang="ts">
	// /deletions — migrated to the EntityPage scaffold; READ-ONLY audit log of soft-deleted content.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye } from 'lucide-svelte';

	type Deletion = {
		id: string;
		user_id: string;
		username?: string;
		display_name?: string;
		content_type: string;
		content_id: string;
		reason: string;
		deleted_at: string;
		hard_deleted: boolean;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// Read-only audit log: no create/edit/delete form. `fields` is required by
	// EntityPageConfig but unused because `readonly: true` hides the form flows.
	const fields: FieldDef[] = [];

	const config: EntityPageConfig = {
		title: t('deletions.title', 'Deletions'),
		description: t('deletions.description', 'GDPR data deletion requests'),
		readonly: true,
		createPermission: 'content.moderate',
		pageSize: 25,
		columns: [
			{
				key: 'display_name',
				label: t('deletions.user', 'User'),
				render: (v: string, row: Deletion) =>
					`<span class="text-sm">${v || row.username || 'anonymous'}</span>`
			},
			{
				key: 'content_type',
				label: t('deletions.type', 'Type'),
				render: (v: string) => `<span class="text-xs">${v}</span>`
			},
			{
				key: 'reason',
				label: t('deletions.reason', 'Reason'),
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'hard_deleted',
				label: t('deletions.hard', 'Hard Delete'),
				render: (v: boolean) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-danger/15 text-danger dark:text-danger' : 'bg-warning/15 text-warning dark:text-warning'}">${v ? t('deletions.yes', 'Yes') : t('deletions.no', 'No')}</span>`
			},
			{
				key: 'deleted_at',
				label: t('deletions.date', 'Date'),
				sortable: true,
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>`
			}
		],
		// Read-only: only a View action (opens the view slide-over). No Edit/Delete/Create.
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (_row: Deletion, ctx) => ctx?.openView(_row)
			}
		],
		fields,
		load: async ({ q }) => {
			try {
				const result = await api<Deletion[]>('/admin/deletions');
				if (result.ok) {
					let data = result.data ?? [];
					// Client-side search (legacy filtered on display_name/username/content_type/reason).
					if (q) {
						const query = q.toLowerCase();
						data = data.filter(
							(d) =>
								(d.display_name || d.username || '').toLowerCase().includes(query) ||
								d.content_type.toLowerCase().includes(query) ||
								(d.reason || '').toLowerCase().includes(query)
						);
					}
					return { data, total: data.length, totalPages: 1 };
				}
				throw new Error(result.error ?? t('common.error', 'Failed to load deletions'));
			} catch (e) {
				pageError = e instanceof Error ? e.message : t('common.error', 'Failed to load deletions');
				throw e;
			}
		},
		viewSnippet: DeletionView
	};
</script>

<svelte:head>
	<title>{t('deletions.title', 'Deletions')} — JapaConnect Admin</title>
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

<!-- View slide-over body: full deletion record -->
{#snippet DeletionView({ row }: { row: Deletion })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.user', 'User')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.display_name || row.username || 'anonymous'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.type', 'Type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.content_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				Content ID
			</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.content_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.reason', 'Reason')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reason || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.date', 'Date')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.deleted_at).toLocaleString()}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.hard', 'Hard Delete')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.hard_deleted ? t('deletions.yes', 'Yes') : t('deletions.no', 'No')}
			</p>
		</div>
	</div>
{/snippet}
