<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /page-builder/themes — migrated to the EntityPage scaffold.
	// Simple CRUD entity using declarative `fields` (EntityForm) plus a custom
	// `actions` array with a "Set Default" state action backed by a page-local
	// confirm Modal (EntityPage's built-in confirm only covers delete).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { pageBuilderAdminApiExtended, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, Star } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type PageTheme = {
		id: string;
		name: string;
		slug: string;
		is_default: boolean;
		created_at: string;
		updated_at: string;
	};

	const canManageThemes = $derived(hasPermission('pagebuilder.themes.manage'));

	// slugify derives a URL-safe slug from a free-form name so the backend
	// (CreateThemeInput.Slug — required) always receives a valid value even
	// when the admin leaves the Slug field blank. Mirrors the legacy submit().
	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
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
			await Promise.all(idArr.map((id) => pageBuilderAdminApiExtended.deleteTheme(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete themes');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Set Default state action ────────────────────────────────────────────
	// EntityPage's confirm Modal only covers default delete; state actions keep
	// a page-local Modal with richer copy, confirm calls API + controller.reload().
	let showSetDefaultConfirm = $state(false);
	let setDefaultTarget = $state<PageTheme | null>(null);
	let settingDefault = $state(false);

	function openSetDefault(theme: PageTheme) {
		setDefaultTarget = theme;
		showSetDefaultConfirm = true;
	}

	async function confirmSetDefault() {
		if (!setDefaultTarget) return;
		settingDefault = true;
		pageError = '';
		try {
			const result = await pageBuilderAdminApiExtended.setDefaultTheme(setDefaultTarget.id);
			if (result.ok) {
				showSetDefaultConfirm = false;
				setDefaultTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to set default theme');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to set default theme');
		} finally {
			settingDefault = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding. Mirrors the
	// legacy form exactly: name (required) + slug (auto-generated when blank).
	const fields: FieldDef[] = [
		{ key: 'name', label: t('themes.name', 'Name'), type: 'text', placeholder: 'e.g., Dark Theme', required: true },
		{
			key: 'slug',
			label: t('themes.slug', 'Slug'),
			type: 'text',
			placeholder: t('themes.slug_placeholder', 'auto-generated-from-name'),
			help: t('themes.slug_hint', 'Leave blank to auto-generate from the name.')
		}
	];

	// Custom `actions` replace the defaults, so View/Edit/Delete are included
	// explicitly alongside the Set Default state action. show() predicates
	// mirror the legacy ThemeActionsCell (Set Default + Delete hidden when the
	// row is already the default theme).
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canManageThemes) {
			acts.push({ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (_row, ctx) => ctx?.openEdit(_row) });
			acts.push({
				label: t('themes.set_default', 'Set Default'),
				icon: Star,
				onclick: (row: PageTheme) => openSetDefault(row),
				show: (row: PageTheme) => !row.is_default
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: (row: PageTheme) => !row.is_default
			});
		}
		return acts;
	});

	// Config built as a $derived so rowActions updates (permission-gated) propagate.
	const config = $derived<EntityPageConfig>({
		title: t('themes.title', 'Page Themes'),
		description: t('themes.description', 'Manage page builder themes'),
		createLabel: t('themes.create', 'Create Theme'),
		createPermission: 'pagebuilder.themes.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('themes.name', 'Name'), sortable: true },
			{ key: 'slug', label: t('themes.slug', 'Slug'), sortable: true },
			{
				key: 'is_default',
				label: t('themes.default', 'Default'),
				sortable: true,
				render: (v: boolean) =>
					v
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-primary/15 text-primary dark:text-primary">${t('common.yes', 'Yes')}</span>`
						: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-muted/15 text-muted dark:text-muted">${t('common.no', 'No')}</span>`
			},
			{
				key: 'updated_at',
				label: t('themes.updated', 'Updated'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			// EntityPage's page is 0-based; the API's `page` is 1-based.
			params.set('page', String(page + 1));
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				if (dir) params.set('dir', dir);
			}
			if (q) params.set('q', q);
			const result = await apiRaw<{ data?: PageTheme[]; total?: number; total_pages?: number }>(
				`/admin/pagebuilder/themes?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				const items = d.data ?? [];
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load themes'));
		},
		create: async (body) => {
			const name = String(body.name ?? '').trim();
			const res = await pageBuilderAdminApiExtended.createTheme({
				name,
				slug: String(body.slug ?? '').trim() || slugify(name)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const name = String(body.name ?? '').trim();
			const res = await pageBuilderAdminApiExtended.updateTheme(id, {
				name,
				slug: String(body.slug ?? '').trim() || slugify(name)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await pageBuilderAdminApiExtended.deleteTheme(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ThemeView
	});
</script>

<svelte:head>
	<title>{t('themes.title', 'Page Themes')} — {siteName()} Admin</title>
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

<!-- ── Set Default confirmation modal ─────────────────────────────────────── -->
<Modal bind:open={showSetDefaultConfirm} title={t('themes.set_default', 'Set Default Theme')}>
	{#if setDefaultTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('themes.set_default_confirm', 'Set')} <strong>{setDefaultTarget.name}</strong> {t('themes.as_default', 'as the default theme?')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showSetDefaultConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmSetDefault} disabled={settingDefault}>
			{settingDefault ? t('common.saving', 'Saving...') : t('themes.set_default', 'Set Default')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View modal body (mirrors the legacy view) -->
{#snippet ThemeView({ row }: { row: PageTheme })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('themes.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('themes.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('themes.default', 'Default')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_default ? t('common.yes', 'Yes') : t('common.no', 'No')}
			</p>
		</div>
		{#if row.created_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('themes.created', 'Created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleDateString()}</p>
			</div>
		{/if}
		{#if row.updated_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('themes.updated', 'Updated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.updated_at).toLocaleDateString()}</p>
			</div>
		{/if}
	</div>
{/snippet}
