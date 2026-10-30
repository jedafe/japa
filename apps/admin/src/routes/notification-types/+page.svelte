<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /notification-types — migrated to the EntityPage scaffold; simple CRUD entity using `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type NotificationType = {
		id: string; // mapped from `key` so EntityPage's id-based update/remove + bulk selection work
		key: string;
		label: string;
		category: string;
		description: string;
		enabled: boolean;
		default_in_app: boolean;
		default_email: boolean;
		default_push: boolean;
	};

	// The backend requires `category` on create AND update (it sets the
	// column unconditionally). These values mirror the seeded categories in
	// migration 000564 (notification_types.category backfill).
	const CATEGORY_OPTIONS = [
		{ value: 'social', label: 'Social' },
		{ value: 'content', label: 'Content' },
		{ value: 'messages', label: 'Messages' },
		{ value: 'account', label: 'Account' },
		{ value: 'billing', label: 'Billing' },
		{ value: 'system', label: 'System' }
	];

	// Normalize an API row: copy `key` into `id` so the scaffold's id-based mutations and bulk selection work.
	function mapType(raw: any): NotificationType {
		return { ...raw, id: raw.key };
	}

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
			await Promise.all(
				idArr.map((id) => api(`/admin/notification-types/${id}`, { method: 'DELETE' }))
			);
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete notification types');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding; `key` is immutable on edit.
	const fields: FieldDef[] = [
		{ key: 'key', label: t('notification_types.key', 'Key'), type: 'text', placeholder: 'new_comment', required: true, readonlyInEdit: true },
		{ key: 'label', label: t('notification_types.label', 'Label'), type: 'text', placeholder: 'New Comment', required: true },
		{ key: 'category', label: t('notification_types.category', 'Category'), type: 'select', options: CATEGORY_OPTIONS, required: true },
		{ key: 'description', label: t('common.description', 'Description'), type: 'textarea' },
		{ key: 'enabled', label: t('notification_types.enabled', 'Enabled'), type: 'checkbox' },
		{ key: 'default_in_app', label: t('notification-types.in_app', 'In-App'), type: 'checkbox' },
		{ key: 'default_email', label: t('notification-types.email', 'Email'), type: 'checkbox' },
		{ key: 'default_push', label: t('notification-types.push', 'Push'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('notification_types.title', 'Notification Types'),
		description: t('notification_types.description', 'Manage notification types and their default delivery channels.'),
		createLabel: t('notification_types.create', 'Create'),
		createPermission: 'notifications.manage',
		pageSize: 25,
		columns: [
			{
				key: 'key',
				label: t('notification_types.key', 'Key'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs">${v}</span>`
			},
			{ key: 'label', label: t('notification_types.label', 'Label'), sortable: true },
			{
				key: 'category',
				label: t('notification_types.category', 'Category'),
				render: (v: string) => `<span class="text-xs">${v || '-'}</span>`
			},
			{
				key: 'description',
				label: t('common.description', 'Description'),
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '-'}</span>`
			},
			{
				key: 'enabled',
				label: t('notification_types.enabled', 'Enabled'),
				sortable: true,
				componentSnippet: EnabledCell
			},
			{
				key: 'default_channels',
				label: t('notification_types.channels', 'Channels'),
				componentSnippet: ChannelsCell
			}
		],
		filters: [
			{
				key: 'enabled',
				label: t('notification_types.enabled', 'Enabled'),
				options: [
					{ value: 'true', label: t('common.status_active', 'Active') },
					{ value: 'false', label: t('common.status_inactive', 'Inactive') }
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
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.enabled) params.set('enabled', filter.enabled);
			// EntityPage emits sort as "key:dir"; split into bare key + dir for the API.
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('dir', sortDir);
			}
			const result = await apiRaw<{ data: NotificationType[]; total: number; total_pages: number }>(
				`/admin/notification-types?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				const items = (d.data as NotificationType[]) ?? [];
				const mapped = items.map(mapType);
				return {
					data: mapped,
					total: d.total ?? mapped.length,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load notification types'));
		},
		create: async (body) => {
			const res = await api('/admin/notification-types', {
				method: 'POST',
				body: {
					key: String(body.key ?? '').trim(),
					label: String(body.label ?? '').trim(),
					category: String(body.category ?? 'system'),
					description: body.description ? String(body.description) : null,
					enabled: body.enabled === undefined ? true : !!body.enabled,
					default_in_app: body.default_in_app === undefined ? true : !!body.default_in_app,
					default_email: !!body.default_email,
					default_push: !!body.default_push
				}
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// `key` is immutable — update omits it (mirrors the legacy save()).
			const res = await api(`/admin/notification-types/${id}`, {
				method: 'PATCH',
				body: {
					label: String(body.label ?? '').trim(),
					category: String(body.category ?? 'system'),
					description: body.description ? String(body.description) : null,
					enabled: !!body.enabled,
					default_in_app: !!body.default_in_app,
					default_email: !!body.default_email,
					default_push: !!body.default_push
				}
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/notification-types/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TypeView
	};
</script>

<svelte:head>
	<title>{t('notification_types.title', 'Notification Types')} — {siteName()} Admin</title>
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

{#snippet EnabledCell({ value }: { value: boolean; row: NotificationType })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value
			? 'bg-success/15 text-success'
			: 'bg-danger/15 text-danger'}"
	>
		{value ? t('common.status_active', 'Active') : t('common.status_inactive', 'Inactive')}
	</span>
{/snippet}

{#snippet ChannelsCell({ row }: { value: any; row: NotificationType })}
	<div class="flex flex-wrap gap-1">
		{#if row.default_in_app}
			<span class="rounded px-1.5 py-0.5 text-xs bg-success/15 text-success"
				>{t('notification-types.in_app', 'In-App')}</span
			>
		{/if}
		{#if row.default_email}
			<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info"
				>{t('notification-types.email', 'Email')}</span
			>
		{/if}
		{#if row.default_push}
			<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info"
				>{t('notification-types.push', 'Push')}</span
			>
		{/if}
		{#if !row.default_in_app && !row.default_email && !row.default_push}
			<span class="text-xs" style="color: var(--color-muted)">-</span>
		{/if}
	</div>
{/snippet}

<!-- View modal body (mirrors the legacy view) -->
{#snippet TypeView({ row }: { row: NotificationType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('notification_types.key', 'Key')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.key}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('notification_types.label', 'Label')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.label}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('notification_types.category', 'Category')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('common.description', 'Description')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('notification_types.enabled', 'Enabled')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.enabled ? t('common.status_active', 'Active') : t('common.status_inactive', 'Inactive')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('notification_types.channels', 'Default Channels')}
			</p>
			<div class="mt-1 flex flex-wrap gap-1">
				{#if row.default_in_app}
					<span class="rounded px-1.5 py-0.5 text-xs bg-success/15 text-success"
						>{t('notification-types.in_app', 'In-App')}</span
					>
				{/if}
				{#if row.default_email}
					<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info"
						>{t('notification-types.email', 'Email')}</span
					>
				{/if}
				{#if row.default_push}
					<span class="rounded px-1.5 py-0.5 text-xs bg-info/15 text-info"
						>{t('notification-types.push', 'Push')}</span
					>
				{/if}
				{#if !row.default_in_app && !row.default_email && !row.default_push}
					<span class="text-sm" style="color: var(--color-muted)">—</span>
				{/if}
			</div>
		</div>
	</div>
{/snippet}
