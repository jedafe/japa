<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { reactionsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type ReactionType = {
		id: string; // mapped from `key` so EntityPage's id-based update/remove + bulk selection work
		key: string;
		label: string;
		emoji: string;
		enabled: boolean;
		position: number;
		created_at: string;
	};

	// Normalize an API row: copy `key` into `id` so the scaffold's id-based mutations and bulk selection work.
	function mapReaction(raw: any): ReactionType {
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
			await Promise.all(idArr.map((id) => reactionsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete reactions');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{ key: 'key', label: t('reactions.key', 'Key'), type: 'text', placeholder: 'like', required: true },
		{ key: 'label', label: t('reactions.name', 'Name'), type: 'text', placeholder: 'Like', required: true },
		{ key: 'emoji', label: t('reactions.emoji', 'Emoji'), type: 'text', placeholder: '👍', required: true },
		{ key: 'enabled', label: t('reactions.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('reactions.title', 'Reactions'),
		description: t('reactions.manage_reaction_types_used_across_the_pl', 'Manage reaction types used across the platform'),
		createLabel: t('reactions.create', 'Create'),
		createPermission: 'content.reaction_types.manage',
		pageSize: 25,
		columns: [
			{ key: 'emoji', label: t('reactions.emoji', 'Emoji'), componentSnippet: EmojiCell },
			{ key: 'label', label: t('reactions.name', 'Name'), sortable: true },
			{
				key: 'key',
				label: t('reactions.key', 'Key'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'enabled', label: t('reactions.active', 'Active'), sortable: true, componentSnippet: StatusCell }
		],
		filters: [
			{
				key: 'enabled',
				label: t('reactions.status', 'Status'),
				options: [
					{ value: 'true', label: t('reactions.active', 'Active') },
					{ value: 'false', label: t('reactions.inactive', 'Inactive') }
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
			// The backend currently returns every reaction type without pagination,
			// so we fetch a large window and apply client-side search/filter/pagination.
			const result = await reactionsAdminApi.list('limit=10000');
			if (result.ok) {
				let data = ((result.data as ReactionType[]) ?? []).map(mapReaction);
				if (filter.enabled) {
					const enabled = filter.enabled === 'true';
					data = data.filter((r) => r.enabled === enabled);
				}
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((r) =>
						r.key.toLowerCase().includes(query) ||
						r.label.toLowerCase().includes(query) ||
						r.emoji.toLowerCase().includes(query)
					);
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load reactions'));
		},
		create: async (body) => {
			const res = await reactionsAdminApi.create({
				key: String(body.key ?? '').trim(),
				label: String(body.label ?? '').trim(),
				emoji: String(body.emoji ?? '').trim(),
				enabled: body.enabled === undefined ? true : !!body.enabled
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// `key` is immutable — update omits it (mirrors the legacy submit()).
			const res = await reactionsAdminApi.update(id, {
				label: String(body.label ?? '').trim(),
				emoji: String(body.emoji ?? '').trim(),
				enabled: !!body.enabled
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await reactionsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ReactionView
	};
</script>

<svelte:head>
	<title>{t('reactions.title', 'Reactions')} — {siteName()} Admin</title>
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

{#snippet EmojiCell({ value }: { value: any; row: ReactionType })}
	<span class="text-2xl">{value}</span>
{/snippet}

{#snippet StatusCell({ value }: { value: boolean; row: ReactionType })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('reactions.active', 'Active') : t('reactions.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View modal body (mirrors the legacy view modal) -->
{#snippet ReactionView({ row }: { row: ReactionType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.emoji', 'Emoji')}</p>
			<p class="mt-1 text-2xl" style="color: var(--color-foreground)">{row.emoji}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.label}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.key', 'Key')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.key}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.enabled ? t('reactions.active', 'Active') : t('reactions.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
