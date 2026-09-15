<script lang="ts">
	// /deletions — read-only audit log of soft-deleted and removed content.
	// Backend: GET /admin/deletions → adminHandlers.ListDeletedContents (RBAC: admin.moderation).
	// Ref: sections/19-gdpr-ndpr-compliance.md, Rule 11 (soft deletes only for user content)
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye } from 'lucide-svelte';

	// Matches gen.ListDeletedContentsRow JSON output.
	type DeletedContent = {
		id: string;
		author_id: string;
		title: string;
		content_type: string;
		status: string;
		deleted_at?: string;
		updated_at: string;
		created_at: string;
		username?: string;
		display_name?: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// Read-only audit log: no create/edit/delete form. `fields` is required by
	// EntityPageConfig but unused because `readonly: true` hides the form flows.
	const fields: FieldDef[] = [];

	const config: EntityPageConfig = {
		title: t('deletions.title', 'Deletions'),
		description: t('deletions.description', 'Soft-deleted and removed content audit log'),
		readonly: true,
		createPermission: 'admin.moderation',
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('deletions.title_col', 'Title'),
				render: (v: string) =>
					`<span class="text-sm">${v || '<em>untitled</em>'}</span>`
			},
			{
				key: 'content_type',
				label: t('deletions.type', 'Type'),
				render: (v: string) => `<span class="text-xs">${v}</span>`
			},
			{
				key: 'display_name',
				label: t('deletions.user', 'Author'),
				render: (v: string, row: DeletedContent) =>
					`<span class="text-sm">${v || row.username || 'anonymous'}</span>`
			},
			{
				key: 'status',
				label: t('deletions.status', 'Status'),
				render: (v: string) => {
					const colors = v === 'removed' ? 'bg-danger/15 text-danger dark:text-danger' : 'bg-warning/15 text-warning dark:text-warning';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
				}
			},
			{
				key: 'deleted_at',
				label: t('deletions.date', 'Deleted At'),
				render: (v: string) =>
					v
						? `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>`
						: `<span class="text-xs" style="color: var(--color-muted)">—</span>`
			}
		],
		// Read-only: only a View action (opens the view slide-over). No Edit/Delete/Create.
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (_row: DeletedContent, ctx) => ctx?.openView(_row)
			}
		],
		fields,
		load: async ({ q, page, pageSize }) => {
			try {
				const params = new URLSearchParams();
				params.set('page_size', String(pageSize));
				params.set('page_offset', String(page * pageSize));
				if (q) params.set('q', encodeURIComponent(q));
				const result = await api<DeletedContent[] | { data?: DeletedContent[]; total?: number }>(`/admin/deletions?${params.toString()}`);
				if (result.ok) {
					const raw = result.data;
					const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
					const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
					const totalPages = Math.max(1, Math.ceil(total / pageSize));
					return { data, total, totalPages };
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

<!-- View slide-over body: full deleted content record -->
{#snippet DeletionView({ row }: { row: DeletedContent })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.title_col', 'Title')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.title || '<em>untitled</em>'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Content ID</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.type', 'Type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.content_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.user', 'Author')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.display_name || row.username || 'anonymous'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Author ID</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.author_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('deletions.date', 'Deleted At')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">
				{row.deleted_at ? new Date(row.deleted_at).toLocaleString() : '—'}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Created At</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
