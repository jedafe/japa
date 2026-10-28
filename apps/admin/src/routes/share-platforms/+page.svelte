<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /share-platforms — migrated to the EntityPage scaffold (Task 10); simple CRUD entity using `formFields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
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

	// Normalize an API row: copy `key` into `id` so the scaffold's id-based mutations and bulk selection work.
	function mapPlatform(raw: any): SharePlatform {
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
			await Promise.all(idArr.map((id) => sharePlatformsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete share platforms');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding; color uses a plain text input (no native color picker), hex string preserved.
	const fields: FieldDef[] = [
		{ key: 'key', label: t('share_platforms.key', 'Key'), type: 'text', help: t('share_platforms.key_hint', 'The machine key used in code and API responses.'), placeholder: 'twitter', required: true },
		{ key: 'label', label: t('share_platforms.label', 'Label'), type: 'text', help: t('share_platforms.label_hint', 'The display label shown to users.'), placeholder: 'Twitter', required: true },
		{ key: 'icon', label: t('share_platforms.icon', 'Icon'), type: 'text', help: t('share_platforms.icon_hint', 'Icon name or emoji shown next to the record.'), placeholder: '🐦', required: true },
		{
			key: 'url_template',
			label: t('share_platforms.url_template', 'URL Template'),
			type: 'text',
			placeholder: "https://twitter.com/intent/tweet?url={url}&text={title}",
			required: true,
			help: t('share_platforms.url_template_hint', 'Use {url} and {title} as placeholders.')
		},
		{ key: 'color', label: t('share_platforms.color', 'Color'), type: 'text', help: t('share_platforms.color_hint', 'The display color for this record.'), placeholder: '#3b82f6' },
		{ key: 'position', label: t('share_platforms.position', 'Position'), type: 'number', help: t('share_platforms.position_hint', 'Ordering position — lower numbers show first.') },
		{ key: 'enabled', label: t('share_platforms.active', 'Enabled'), type: 'checkbox', help: t('share_platforms.enabled_hint', 'Turn off to disable without deleting.') }
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
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'label', label: t('share_platforms.label', 'Label') },
			{ key: 'icon', label: t('share_platforms.icon', 'Icon'), componentSnippet: IconCell },
			{
				key: 'url_template',
				label: t('share_platforms.url_template', 'URL Template'),
				render: (v: string) => `<span class="font-mono text-xs truncate" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'color', label: t('share_platforms.color', 'Color'), componentSnippet: ColorCell },
			{ key: 'enabled', label: t('share_platforms.active', 'Enabled'), componentSnippet: StatusCell },
			{ key: 'position', label: t('share_platforms.position', 'Position') }
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
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			// The backend returns every share platform without pagination,
			// so we fetch a large window and apply client-side search/filter/pagination.
			const result = await sharePlatformsAdminApi.list();
			if (result.ok) {
				let data = ((result.data as SharePlatform[]) ?? []).map(mapPlatform);
				if (filter.enabled) {
					const enabled = filter.enabled === 'true';
					data = data.filter((p) => p.enabled === enabled);
				}
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((p) =>
						p.key.toLowerCase().includes(query) ||
						p.label.toLowerCase().includes(query) ||
						p.url_template.toLowerCase().includes(query)
					);
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
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
				enabled: body.enabled === undefined ? true : !!body.enabled,
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
	<title>{t('share_platforms.title', 'Share Platforms')} — {siteName()} Admin</title>
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
