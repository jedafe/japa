<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /docs/categories — manages the public docs.categories setting (the
	// docs portal's category/subcategory catalog). Lives on its own subpage
	// instead of Settings → Documentation because it is docs-surface work.
	// The /admin/settings/* endpoints require admin.settings server-side, so
	// the page renders an error rather than a broken editor without it.
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardHeader, CardContent } from '$lib/components/ui/card';
	import DocsCategoriesEditor from '$lib/components/settings/DocsCategoriesEditor.svelte';
	import { api, settingsAdminApi } from '$lib/api';
	import { ArrowLeft } from 'lucide-svelte';
	import { onMount } from 'svelte';

	const canManage = $derived(hasPermission('admin.settings'));

	let loaded = $state(false);
	let loadError = $state('');
	let value = $state<unknown>([]);
	let original = '';
	let saving = $state(false);
	let saveMsg = $state('');
	const dirty = $derived(loaded && JSON.stringify(value ?? []) !== original);

	onMount(async () => {
		if (!canManage) return;
		// docs.categories is a public setting — read it via the public
		// endpoint so the admin settings list doesn't have to expose it.
		const result = await api<Record<string, unknown>>('/site/settings/public');
		if (!result.ok) {
			loadError = result.error;
			loaded = true;
			return;
		}
		value = result.data?.['docs.categories'] ?? [];
		original = JSON.stringify(value ?? []);
		loaded = true;
	});

	async function save() {
		saving = true;
		saveMsg = '';
		loadError = '';
		// PUT upserts only the provided keys — other docs.* settings are untouched.
		const result = await settingsAdminApi.updateCategory('docs', { categories: value ?? [] });
		if (result.ok) {
			original = JSON.stringify(value ?? []);
			saveMsg = t('docs.categories_saved', 'Categories saved.');
			setTimeout(() => {
				saveMsg = '';
			}, 4000);
		} else {
			loadError = result.error;
		}
		saving = false;
	}
</script>

<svelte:head>
	<title>{t('docs.categories_title', 'Categories & Subcategories')} — {siteName()} Admin</title>
</svelte:head>

<a
	href="/docs"
	class="mb-4 inline-flex items-center gap-1.5 text-sm"
	style="color: var(--color-muted)"
>
	<ArrowLeft class="h-3.5 w-3.5 rtl:-scale-x-100" />
	{t('docs.back_to_docs', 'Back to Documentation')}
</a>

<PageHeader
	title={t('docs.categories_title', 'Categories & Subcategories')}
	description={t('docs.categories_desc', 'Managed docs-portal catalog: display order, card descriptions/icons, and canonical subcategory names. Page-level category fields stay free-text; categories already in use are merged in automatically.')}
/>

{#if !canManage}
	<div
		class="rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
	>
		{t('docs.categories_no_permission', 'You need the admin.settings permission to manage documentation categories.')}
	</div>
{:else}
	<Card>
		<CardHeader>
			<div class="flex flex-wrap items-start justify-between gap-3">
				<div class="min-w-0 flex-1">
					<h2 class="text-base font-semibold">{t('docs.categories_catalog', 'Category Catalog')}</h2>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">
						{t('docs.categories_catalog_desc', 'Add, reorder, and describe categories and their subcategories. Icons are optional — “auto” lets the portal pick one.')}
					</p>
				</div>
				<Button size="sm" onclick={save} disabled={saving || !dirty}>
					{saving ? t('docs.saving') : t('common.save', 'Save')}
				</Button>
			</div>
		</CardHeader>
		<CardContent>
			{#if loadError}
				<div class="mb-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
					{loadError}
				</div>
			{/if}
			{#if saveMsg}
				<div class="mb-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
					{saveMsg}
				</div>
			{/if}
			{#if !loaded}
				<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
			{:else}
				<DocsCategoriesEditor
					{value}
					disabled={saving}
					maxItems={24}
					onchange={(next) => { value = next; }}
				/>
			{/if}
		</CardContent>
	</Card>
{/if}
