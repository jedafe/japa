<script lang="ts">
	// /redirects — migrated to the EntityPage scaffold; simple CRUD entity using declarative `fields` (EntityForm).
	// Ref: japaconnect-engineering/adr/ADR-056-url-redirects.md
	// Field names mirror the backend gen.Redirect JSON contract:
	// source_path, target_url, http_status, is_active, notes, hit_count, last_hit_at.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { redirectsAdminApi } from '$lib/api';
	import { apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type Redirect = {
		id: string;
		tenant_id: string;
		source_path: string;
		target_url: string;
		http_status: number;
		is_active: boolean;
		notes: string | null;
		hit_count: number;
		last_hit_at: string | null;
		created_at: string;
		updated_at: string;
	};

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => redirectsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete redirects');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{
			key: 'source_path',
			label: t('redirects.from', 'From Path'),
			type: 'text',
			placeholder: '/old-page',
			required: true,
			help: t('redirects.invalid_source', 'From path must start with /')
		},
		{
			key: 'target_url',
			label: t('redirects.to', 'To URL'),
			type: 'text',
			placeholder: '/new-page or https://example.com/path',
			required: true,
			help: t('redirects.invalid_target', 'To URL must be a root-relative path or an absolute http/https URL')
		},
		{
			key: 'http_status',
			label: t('redirects.code', 'Status Code'),
			type: 'select',
			options: [
				{ value: '301', label: `301 ${t('redirects.permanent', 'Permanent')}` },
				{ value: '302', label: `302 ${t('redirects.temporary', 'Temporary')}` },
				{ value: '307', label: `307 ${t('redirects.temp_redirect', 'Temporary Redirect')}` }
			]
		},
		{ key: 'is_active', label: t('common.active', 'Active'), type: 'checkbox' },
		{
			key: 'notes',
			label: t('redirects.notes', 'Notes'),
			type: 'textarea',
			placeholder: t('redirects.notes_placeholder', 'Optional internal note')
		}
	];

	const config: EntityPageConfig = {
		title: t('redirects.title', 'Redirects'),
		description: t('redirects.description', 'Manage URL redirects'),
		createLabel: t('redirects.create', 'Create Redirect'),
		createPermission: 'redirects.manage',
		pageSize: 25,
		columns: [
			{
				key: 'source_path',
				label: t('redirects.from', 'From'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'target_url',
				label: t('redirects.to', 'To'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs truncate" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'http_status', label: t('redirects.code', 'Code'), sortable: true },
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				sortable: true,
				componentSnippet: StatusCell
			},
			{ key: 'hit_count', label: t('redirects.hits', 'Hits'), sortable: true }
		],
		filters: [
			{
				key: 'status',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'active', label: t('common.active', 'Active') },
					{ value: 'inactive', label: t('common.inactive', 'Inactive') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			// Use apiRaw to access top-level total/total_pages alongside data (matches tags pattern).
			const result = await apiRaw<{ data: Redirect[]; total: number; total_pages: number }>(
				`/admin/redirects${params.toString() ? `?${params.toString()}` : ''}`
			);
			if (result.ok) {
				const body = result.data;
				const items = Array.isArray(body?.data) ? body.data : [];
				const total = body?.total ?? items.length;
				const totalPages = body?.total_pages ?? Math.max(1, Math.ceil(total / pageSize));
				return { data: items, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load redirects'));
		},
		create: async (body) => {
			// Create payload omits is_active (DB defaults to true); mirrors the legacy submit().
			const res = await redirectsAdminApi.create({
				source_path: String(body.source_path ?? '').trim(),
				target_url: String(body.target_url ?? '').trim(),
				http_status: Number(body.http_status ?? 301),
				notes: String(body.notes ?? '').trim() || null
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// Update includes is_active (mirrors the legacy submit()).
			const res = await redirectsAdminApi.update(id, {
				source_path: String(body.source_path ?? '').trim(),
				target_url: String(body.target_url ?? '').trim(),
				http_status: Number(body.http_status ?? 301),
				is_active: !!body.is_active,
				notes: String(body.notes ?? '').trim() || null
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await redirectsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: RedirectView
	};
</script>

<svelte:head>
	<title>{t('redirects.title', 'Redirects')} — JapaConnect Admin</title>
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

{#snippet StatusCell({ value }: { value: boolean; row: Redirect })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value
			? 'bg-success/15 text-success'
			: 'bg-muted/15 text-muted'}"
	>
		{value ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view) -->
{#snippet RedirectView({ row }: { row: Redirect })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('redirects.from', 'From Path')}
			</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.source_path}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('redirects.to', 'To URL')}
			</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.target_url}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('redirects.code', 'Status Code')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.http_status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('redirects.hits', 'Hits')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hit_count}</p>
		</div>
		{#if row.notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('redirects.notes', 'Notes')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.notes}</p>
			</div>
		{/if}
		{#if row.last_hit_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('redirects.last_hit', 'Last Hit')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.last_hit_at}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.created_at', 'Created')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at}</p>
		</div>
	</div>
{/snippet}
