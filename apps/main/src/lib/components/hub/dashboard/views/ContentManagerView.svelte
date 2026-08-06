<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { publishContent, archiveContent, deleteContent } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidate } from '$app/navigation';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { FileText } from 'lucide-svelte';

	interface ContentItem {
		id: string;
		title?: string;
		content_type?: string;
		status?: string;
		created_at?: string;
		views?: number;
		comments?: number;
	}

	interface ContentTypeOption {
		value: string;
		label: string;
	}

	interface ContentManagerData {
		items: ContentItem[];
		contentTypes: ContentTypeOption[];
	}

	let { data }: { data: unknown } = $props();
	const viewData = $derived(data as ContentManagerData | null);
	let items = $state<ContentItem[]>(untrack(() => viewData?.items ?? []));
	const contentTypes = $derived(viewData?.contentTypes ?? []);

	const status = $derived(page.url.searchParams.get('status') ?? 'published');
	const typeFilter = $derived(page.url.searchParams.get('type') ?? '');

	const tabs: { key: string; labelKey: string }[] = [
		{ key: 'published', labelKey: 'dashboard.content.tab_published' },
		{ key: 'draft', labelKey: 'dashboard.content.tab_drafts' },
		{ key: 'scheduled', labelKey: 'dashboard.content.tab_scheduled' }
	];

	function switchTab(key: string) {
		const params = new URLSearchParams(page.url.searchParams);
		if (key === 'published') params.delete('status');
		else params.set('status', key);
		goto(`/dashboard/content?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function setType(type: string) {
		const params = new URLSearchParams(page.url.searchParams);
		if (type) params.set('type', type);
		else params.delete('type');
		goto(`/dashboard/content?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	// Format the content type for display — use the API label if available,
	// otherwise fall back to the raw value.
	function typeLabel(type: string | undefined): string {
		if (!type) return t('dashboard.content.all_types');
		const found = contentTypes.find((ct) => ct.value === type);
		return found?.label ?? type;
	}

	// Format an ISO date string as a locale-aware date.
	function formatDate(iso: string | undefined): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleDateString(i18n.locale);
	}

	let deleteTarget = $state<string | null>(null);

	async function doPublish(id: string) {
		await optimisticAction({
			apply: () => (items = items.map((i) => (i.id === id ? { ...i, status: 'published' } : i))),
			rollback: () => (items = viewData?.items ?? []),
			call: () => publishContent(id)
		});
		await invalidate('/api/v1/me/content');
	}

	async function doArchive(id: string) {
		await optimisticAction({
			apply: () => (items = items.map((i) => (i.id === id ? { ...i, status: 'archived' } : i))),
			rollback: () => (items = viewData?.items ?? []),
			call: () => archiveContent(id)
		});
		await invalidate('/api/v1/me/content');
	}

	async function doDelete() {
		if (!deleteTarget) return;
		const id = deleteTarget;
		deleteTarget = null;
		await optimisticAction({
			apply: () => (items = items.filter((i) => i.id !== id)),
			rollback: () => (items = viewData?.items ?? []),
			call: () => deleteContent(id)
		});
		await invalidate('/api/v1/me/content');
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.content')}</h1>
	</header>

	<div class="flex flex-wrap items-center gap-2">
		{#each tabs as tab (tab.key)}
			<button
				type="button"
				class="rounded-[var(--radius)] px-3 py-1.5 text-sm {status === tab.key || (tab.key === 'published' && !page.url.searchParams.get('status'))
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				onclick={() => switchTab(tab.key)}
			>
				{t(tab.labelKey)}
			</button>
		{/each}
		<select
			class="ml-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]"
			value={typeFilter}
			onchange={(e) => setType(e.currentTarget.value)}
		>
			<option value="">{t('dashboard.content.all_types')}</option>
			{#each contentTypes as ct (ct.value)}
				<option value={ct.value}>{ct.label}</option>
			{/each}
		</select>
	</div>

	{#if items.length === 0}
		<EmptyState icon={FileText} title={t('dashboard.content.empty_title')} body={t('dashboard.content.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each items as item (item.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="min-w-0">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">{item.title ?? t('dashboard.content.untitled')}</p>
						<p class="text-xs text-[var(--color-muted)]">
							{typeLabel(item.content_type)} · {formatDate(item.created_at)}
							{#if item.views !== undefined} · {item.views} {t('dashboard.content.views')}{/if}
						</p>
					</div>
					<div class="ml-3 flex shrink-0 gap-2">
						<a href="/create?draft={item.id}" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
							{t('dashboard.content.edit')}
						</a>
						{#if item.status === 'draft'}
							<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doPublish(item.id)}>
								{t('dashboard.content.publish')}
							</button>
						{/if}
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doArchive(item.id)}>
							{t('dashboard.content.archive')}
						</button>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => (deleteTarget = item.id)}>
							{t('dashboard.content.delete')}
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<Dialog.Root open={deleteTarget !== null} onOpenChange={(v) => { if (!v) deleteTarget = null; }}>
		<Dialog.Portal>
			<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
				<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">{t('dashboard.content.delete_confirm')}</Dialog.Title>
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">{t('dashboard.content.delete_confirm_body')}</Dialog.Description>
				<div class="flex justify-end gap-2">
					<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">{t('common.cancel')}</Dialog.Close>
					<button type="button" class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white" onclick={doDelete}>{t('dashboard.content.delete')}</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
