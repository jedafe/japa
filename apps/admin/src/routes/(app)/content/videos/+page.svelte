<script lang="ts">
	// /content/videos — admin DataTable for video_sources.
	// Ref: japaconnect-engineering/sections/106-video-content-and-import.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { videoSourcesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type VideoSource = {
		id: string;
		tenant_id: string;
		author_user_id: string;
		author_username?: string;
		source_type: string;
		source_url: string;
		external_id?: string;
		title: string;
		description?: string;
		category?: string;
		thumbnail_url?: string;
		thumbnail_media_id?: string;
		duration_secs?: number;
		status: string;
		field_values?: Record<string, unknown>;
		created_at: string;
		updated_at: string;
	};

	function mapSource(raw: any): VideoSource {
		return { ...raw, id: raw.id?.toString?.() ?? raw.id };
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	function formatDuration(seconds?: number): string {
		if (seconds == null || seconds <= 0) return '-';
		const m = Math.floor(seconds / 60);
		const s = Math.floor(seconds % 60);
		return `${m}:${s.toString().padStart(2, '0')}`;
	}

	const fields: FieldDef[] = [
		{
			key: 'status',
			label: t('video_sources.status', 'Status'),
			type: 'select',
			options: [
				{ value: 'active', label: t('video_sources.status_active', 'Active') },
				{ value: 'removed', label: t('video_sources.status_removed', 'Removed') }
			],
			required: true
		}
	];

	const config: EntityPageConfig = {
		title: t('video_sources.title', 'Video Sources'),
		description: t('video_sources.description', 'Manage imported and uploaded video sources.'),
		createPermission: 'admin.video_sources.create',
		editPermission: 'admin.video_sources.manage',
		deletePermission: 'admin.video_sources.manage',
		restorePermission: 'admin.video_sources.manage',
		pageSize: 25,
		isDeleted: (row: any) => row?.status === 'removed',
		columns: [
			{
				key: 'thumbnail_url',
				label: t('video_sources.thumbnail', 'Thumbnail'),
				class: 'w-24',
				sortable: false,
				render: (v: string) =>
					v
						? `<img src="${v}" class="h-10 w-16 rounded object-cover" loading="lazy" alt="" />`
						: `<span class="text-xs text-muted">-</span>`
			},
			{ key: 'title', label: t('video_sources.title', 'Title'), sortable: true },
			{ key: 'source_type', label: t('video_sources.source_type', 'Type'), sortable: true },
			{ key: 'author_username', label: t('video_sources.author', 'Author'), sortable: false },
			{
				key: 'status',
				label: t('video_sources.status', 'Status'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${
						v === 'active'
							? 'bg-success/10 text-success'
							: 'bg-danger/10 text-danger'
					}">${v}</span>`
			},
			{ key: 'duration_secs', label: t('video_sources.duration', 'Duration'), sortable: false, render: (v: number) => formatDuration(v) },
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleString() : '-')
			}
		],
		filters: [
			{
				key: 'status',
				label: t('video_sources.status', 'Status'),
				options: [
					{ value: 'active', label: t('video_sources.status_active', 'Active') },
					{ value: 'removed', label: t('video_sources.status_removed', 'Removed') }
				]
			},
			{
				key: 'source_type',
				label: t('video_sources.source_type', 'Type'),
				options: [
					{ value: 'youtube', label: 'YouTube' },
					{ value: 'vimeo', label: 'Vimeo' },
					{ value: 'upload', label: t('video_sources.type_upload', 'Upload') }
				]
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.source_type) params.set('source_type', filter.source_type);
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('dir', sortDir);
			}
			const result = await videoSourcesAdminApi.list(params.toString());
			if (result.ok) {
				const d = (result.data as { items: any[]; total: number; total_pages: number }) ?? { items: [], total: 0, total_pages: 1 };
				const data = (d.items ?? []).map(mapSource);
				return { data, total: d.total ?? data.length, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load video sources'));
		},
		update: async (id, body) => {
			const res = await videoSourcesAdminApi.updateStatus(id, { status: String(body.status ?? 'active') });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await videoSourcesAdminApi.updateStatus(id, { status: 'removed' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		restore: async (id) => {
			const res = await videoSourcesAdminApi.updateStatus(id, { status: 'active' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SourceView
	};
</script>

<svelte:head>
	<title>{t('video_sources.title', 'Video Sources')} — JapaConnect Admin</title>
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

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet SourceView({ row }: { row: VideoSource })}
	<div class="space-y-4">
		{#if row.thumbnail_url}
			<img src={row.thumbnail_url} alt={row.title} class="h-32 w-auto rounded object-cover" />
		{/if}
		<div>
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{row.title || t('video_sources.untitled', 'Untitled')}</h3>
			<p class="text-sm text-muted">{row.source_type} — {row.source_url}</p>
		</div>
		<dl class="grid grid-cols-2 gap-2 text-sm">
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_sources.author', 'Author')}</dt>
				<dd>{row.author_username || row.author_user_id}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_sources.status', 'Status')}</dt>
				<dd class="capitalize">{row.status}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_sources.duration', 'Duration')}</dt>
				<dd>{formatDuration(row.duration_secs)}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('common.created', 'Created')}</dt>
				<dd>{row.created_at ? new Date(row.created_at).toLocaleString() : '-'}</dd>
			</div>
			{#if row.category}
				<div class="col-span-2">
					<dt class="text-muted">{t('video_sources.category', 'Category')}</dt>
					<dd>{row.category}</dd>
				</div>
			{/if}
			{#if row.description}
				<div class="col-span-2">
					<dt class="text-muted">{t('video_sources.description', 'Description')}</dt>
					<dd class="whitespace-pre-wrap">{row.description}</dd>
				</div>
			{/if}
		</dl>
	</div>
{/snippet}
