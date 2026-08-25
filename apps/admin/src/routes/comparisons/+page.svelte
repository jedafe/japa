<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { comparisonAdminApi, type ComparisonEntry, type ComparisonType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2 } from 'lucide-svelte';
	import type { FilterDef, BulkAction } from '$lib/components/ui/data-table';
	import type { Snippet } from 'svelte';

	type Comparison = ComparisonEntry;

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	let types = $state<ComparisonType[]>([]);
	let visibilityOptions = $state<{ value: string; label: string }[]>([]);
	let statusOptions = $state<{ value: string; label: string }[]>([]);

	const canCreateComparison = $derived(hasPermission('comparison.create'));
	const canEditComparison = $derived(
		hasPermission('comparison.edit_any') || hasPermission('comparison.edit_own')
	);
	const canDeleteComparison = $derived(
		hasPermission('comparison.delete_any') || hasPermission('comparison.delete_own')
	);

	async function loadOptions() {
		const [typesResult, visResult, statusResult] = await Promise.all([
			comparisonAdminApi.listTypes(),
			comparisonAdminApi.getVisibilityOptions(),
			comparisonAdminApi.getStatusOptions()
		]);

		if (typesResult.ok) {
			types = (typesResult.data as ComparisonType[]) ?? [];
		}

		if (visResult.ok && visResult.data) {
			visibilityOptions = (visResult.data as { value: string; label: string }[]).map((o) => ({
				value: o.value,
				label: t(`comparisons.vis_${o.value}`, o.label)
			}));
		}

		if (statusResult.ok && statusResult.data) {
			statusOptions = (statusResult.data as { value: string; label: string }[]).map((o) => ({
				value: o.value,
				label: t(`comparisons.status_${o.value}`, o.label)
			}));
		}
	}

	$effect(() => {
		loadOptions();
	});

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	const typeOptions = $derived([
		{ value: '', label: t('comparisons.type_none', 'No type (generic)') },
		...types.map((tp) => ({ value: tp.id, label: tp.name }))
	]);

	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('comparisons.col_title', 'Title'), type: 'text', required: true },
		{ key: 'slug', label: t('comparisons.col_slug', 'Slug'), type: 'text', required: true },
		{
			key: 'type_id',
			label: t('comparisons.type', 'Comparison Type'),
			type: 'select',
			options: typeOptions,
			readonlyInEdit: true
		},
		{
			key: 'visibility',
			label: t('comparisons.col_visibility', 'Visibility'),
			type: 'select',
			options: visibilityOptions.length ? visibilityOptions : [{ value: 'members', label: 'Members' }],
			required: true
		},
		{
			key: 'status',
			label: t('comparisons.col_status', 'Status'),
			type: 'select',
			options: statusOptions.length
				? statusOptions
				: [
						{ value: 'draft', label: 'Draft' },
						{ value: 'published', label: 'Published' }
			  ]
		},
		{ key: 'body', label: t('comparisons.body', 'Body'), type: 'textarea' }
	]);

	const filters = $derived<FilterDef[]>([
		{
			key: 'visibility',
			label: t('comparisons.col_visibility', 'Visibility'),
			options: visibilityOptions.length
				? visibilityOptions
				: [
						{ value: 'public', label: t('comparisons.vis_public', 'Public') },
						{ value: 'members', label: t('comparisons.vis_members', 'Members') },
						{ value: 'private', label: t('comparisons.vis_private', 'Private') }
				  ]
		},
		{
			key: 'status',
			label: t('comparisons.col_status', 'Status'),
			options: statusOptions.length
				? statusOptions
				: [
						{ value: 'draft', label: 'Draft' },
						{ value: 'published', label: 'Published' }
				  ]
		},
		{
			key: 'depth',
			label: t('comparisons.col_depth', 'Depth'),
			options: [
				{ value: 'quick', label: t('comparisons.depth_quick', 'Quick') },
				{ value: 'detailed', label: t('comparisons.depth_detailed', 'Detailed') }
			]
		}
	]);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;

		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => comparisonAdminApi.deleteComparison(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete comparisons');
		} finally {
			bulkDeleting = false;
		}
	}

	const config = $derived<EntityPageConfig>({
		title: t('comparisons.title', 'Comparisons'),
		description: t('comparisons.subtitle', 'Manage comparison content entries'),
		createLabel: t('comparisons.create', 'Create Comparison'),
		createPermission: canCreateComparison ? 'comparison.create' : undefined,
		editPermission: canEditComparison ? 'comparison.edit_any' : undefined,
		deletePermission: canDeleteComparison ? 'comparison.delete_any' : undefined,
		pageSize: 100,
		columns: [
			{ key: 'title', label: t('comparisons.col_title', 'Title') },
			{
				key: 'slug',
				label: t('comparisons.col_slug', 'Slug'),
				render: (v: string) =>
					`<span class="text-xs font-mono" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'visibility',
				label: t('comparisons.col_visibility', 'Visibility'),
				render: (v: string) => {
					const opt = visibilityOptions.find((o) => o.value === v);
					const label = opt?.label ?? v;
					const color =
						v === 'public'
							? 'bg-success/15 text-success dark:text-success'
							: v === 'members'
								? 'bg-muted/15 text-muted dark:text-muted'
								: 'bg-danger/15 text-danger dark:text-danger';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${label}</span>`;
				}
			},
			{
				key: 'status',
				label: t('comparisons.col_status', 'Status'),
				render: (v: string) => {
					const color =
						v === 'published'
							? 'bg-success/15 text-success dark:text-success'
							: 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${v}</span>`;
				}
			},
			{
				key: 'depth',
				label: t('comparisons.col_depth', 'Depth'),
				render: (v: string) => {
					const color =
						v === 'detailed'
							? 'bg-success/15 text-success dark:text-success'
							: 'bg-muted/15 text-muted dark:text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${color}">${v ?? '—'}</span>`;
				}
			},
			{
				key: 'promoted_from_id',
				label: t('comparisons.col_promoted_from', 'Promoted From'),
				render: (v: string | null) => {
					if (!v) return '—';
					return `<a href="?q=${encodeURIComponent(v)}" class="text-xs font-mono" style="color: var(--color-primary)">${v}</a>`;
				}
			},
			{
				key: 'created_at',
				label: t('comparisons.col_created', 'Created'),
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams({ page_size: '100' });
			if (filter.depth) params.set('depth', filter.depth);
			if (filter.status) params.set('status', filter.status);
			if (filter.visibility) params.set('visibility', filter.visibility);

			const result = await comparisonAdminApi.listComparisonsAdmin(params.toString());
			if (!result.ok) {
				throw new Error(result.error ?? t('common.error', 'Failed to load comparisons'));
			}

			let rows = (result.data as Comparison[]) ?? [];

			if (q) {
				const qLower = q.toLowerCase();
				rows = rows.filter(
					(c) =>
						c.title.toLowerCase().includes(qLower) ||
						c.slug.toLowerCase().includes(qLower)
				);
			}

			if (filter.visibility) {
				rows = rows.filter((c) => c.visibility === filter.visibility);
			}

			if (filter.status) {
				rows = rows.filter((c) => c.status === filter.status);
			}

			if (filter.depth) {
				rows = rows.filter((c) => c.depth === filter.depth);
			}

			return rows;
		},
		create: async (body) => {
			const title = String(body.title ?? '').trim();
			const slug = String(body.slug ?? '').trim() || slugify(title);
			const res = await comparisonAdminApi.createComparison({
				title,
				slug,
				body: String(body.body ?? '').trim() || undefined,
				visibility: String(body.visibility ?? 'members'),
				type_id: String(body.type_id ?? '') || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const title = String(body.title ?? '').trim();
			const slug = String(body.slug ?? '').trim() || slugify(title);
			const res = await comparisonAdminApi.updateComparison(id, {
				title,
				slug,
				body: String(body.body ?? '').trim() || undefined,
				visibility: String(body.visibility ?? 'members')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await comparisonAdminApi.deleteComparison(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ComparisonView
	});
</script>

<svelte:head>
	<title>{t('comparisons.title', 'Comparisons')} — JapaConnect Admin</title>
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

<!-- View slide-over body -->
{#snippet ComparisonView({ row }: { row: Comparison })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_title', 'Title')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_depth', 'Depth')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.depth ?? '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_slug', 'Slug')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_visibility', 'Visibility')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{visibilityOptions.find((o) => o.value === row.visibility)?.label ?? row.visibility}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparisons.col_status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		{#if row.promoted_from_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.col_promoted_from', 'Promoted From')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.promoted_from_id}</p>
			</div>
		{/if}
		{#if row.body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparisons.body', 'Body')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.body}</p>
			</div>
		{/if}
	</div>
{/snippet}
