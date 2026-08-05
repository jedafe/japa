<script lang="ts">
	// /share-platforms — migrated to the EntityPage scaffold (Task 10).
	//
	// This is the "simple path" validation: a straightforward CRUD entity with no
	// media, no sub-flows, and no rich form — so it uses `formFields` (EntityForm)
	// rather than the `formSnippet` escape hatch used by /tags.
	import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
	import { sharePlatformsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type SharePlatform = {
		id: string; // mapped from `key` so EntityPage's id-based update/remove + bulk selection work
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
		enabled: boolean;
		position: number;
		created_at: string;
	};

	// Normalize an API row: copy `key` into `id` so the scaffold's id-based
	// mutations (update/remove) and the DataTable's bulk selection both work.
	function mapPlatform(raw: any): SharePlatform {
		return { ...raw, id: raw.key };
	}

	// ── Bulk delete ─────────────────────────────────────────────────────────
	// EntityPage does not auto-reload after a bulk action, so a remount key
	// forces the scaffold to refetch (and clears the row selection) once done.
	let reloadKey = $state(0);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => sharePlatformsAdminApi.delete(id)));
			reloadKey++;
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete share platforms');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives both EntityForm rendering and the edit-form seeding.
	// The color field uses a plain text input (EntityForm has no native color
	// picker); the hex string is preserved so behavior is unchanged.
	const fields: FieldDef[] = [
		{ key: 'key', label: t('share_platforms.key', 'Key'), type: 'text', placeholder: 'twitter', required: true },
		{ key: 'label', label: t('share_platforms.label', 'Label'), type: 'text', placeholder: 'Twitter', required: true },
		{ key: 'icon', label: t('share_platforms.icon', 'Icon'), type: 'text', placeholder: '🐦', required: true },
		{
			key: 'url_template',
			label: t('share_platforms.url_template', 'URL Template'),
			type: 'text',
			placeholder: "https://twitter.com/intent/tweet?url={url}&text={title}",
			required: true,
			help: t('share_platforms.url_template_hint', 'Use {url} and {title} as placeholders.')
		},
		{ key: 'color', label: t('share_platforms.color', 'Color'), type: 'text', placeholder: '#3b82f6' },
		{ key: 'position', label: t('share_platforms.position', 'Position'), type: 'number' },
		{ key: 'enabled', label: t('share_platforms.active', 'Enabled'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('share_platforms.title', 'Share Platforms'),
		description: t('share_platforms.description', 'Manage share platforms available across the platform for content sharing.'),
		createLabel: t('share_platforms.create', 'Create'),
		createPermission: 'content.reaction_types.manage',
		pageSize: 25,
		columns: [
			{
				key: 'key',
				label: t('share_platforms.key', 'Key'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'label', label: t('share_platforms.label', 'Label'), sortable: true },
			{ key: 'icon', label: t('share_platforms.icon', 'Icon'), componentSnippet: IconCell },
			{
				key: 'url_template',
				label: t('share_platforms.url_template', 'URL Template'),
				render: (v: string) => `<span class="font-mono text-xs truncate" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'color', label: t('share_platforms.color', 'Color'), componentSnippet: ColorCell },
			{ key: 'enabled', label: t('share_platforms.active', 'Enabled'), sortable: true, componentSnippet: StatusCell },
			{ key: 'position', label: t('share_platforms.position', 'Position'), sortable: true }
		],
		filters: [
			{
				key: 'enabled',
				label: t('share_platforms.status', 'Status'),
				options: [
					{ value: 'true', label: t('share_platforms.active', 'Active') },
					{ value: 'false', label: t('share_platforms.inactive', 'Inactive') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2 as any,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.enabled) params.set('enabled', filter.enabled);
			if (q) params.set('q', q);
			const result = await sharePlatformsAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as SharePlatform[]) ?? [];
				const mapped = data.map(mapPlatform);
				return { data: mapped, total: mapped.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load share platforms'));
		},
		create: async (body) => {
			const res = await sharePlatformsAdminApi.create({
				key: String(body.key ?? '').trim(),
				label: String(body.label ?? '').trim(),
				icon: String(body.icon ?? '').trim(),
				url_template: String(body.url_template ?? '').trim(),
				color: String(body.color ?? '#3b82f6'),
				enabled: !!body.enabled,
				position: Number(body.position ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// `key` is immutable — update omits it (mirrors the legacy submit()).
			const res = await sharePlatformsAdminApi.update(id, {
				label: String(body.label ?? '').trim(),
				icon: String(body.icon ?? '').trim(),
				url_template: String(body.url_template ?? '').trim(),
				color: String(body.color ?? '#3b82f6'),
				enabled: !!body.enabled,
				position: Number(body.position ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await sharePlatformsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: PlatformView
	};
</script>

<svelte:head>
	<title>{t('share_platforms.title', 'Share Platforms')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

{#key reloadKey}
	<EntityPage {config} />
{/key}

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet IconCell({ value }: { value: any; row: SharePlatform })}
	<span class="text-xl">{value}</span>
{/snippet}

{#snippet ColorCell({ value }: { value: string; row: SharePlatform })}
	<div class="flex items-center gap-2">
		<span class="inline-block h-4 w-4 rounded-full border" style="background-color: {value}; border-color: var(--color-border)"></span>
		<span class="font-mono text-xs" style="color: var(--color-muted)">{value}</span>
	</div>
{/snippet}

{#snippet StatusCell({ value }: { value: boolean; row: SharePlatform })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('share_platforms.active', 'Active') : t('share_platforms.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet PlatformView({ row }: { row: SharePlatform })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.icon', 'Icon')}</p>
			<p class="mt-1 text-2xl" style="color: var(--color-foreground)">{row.icon}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.label', 'Label')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.label}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.key', 'Key')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.key}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.url_template', 'URL Template')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.url_template}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.color', 'Color')}</p>
			<div class="mt-1 flex items-center gap-2">
				<span class="inline-block h-5 w-5 rounded-full border" style="background-color: {row.color}; border-color: var(--color-border)"></span>
				<span class="text-sm font-mono" style="color: var(--color-foreground)">{row.color}</span>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.position', 'Position')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.position}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('share_platforms.active', 'Enabled')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.enabled ? t('share_platforms.active', 'Active') : t('share_platforms.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
