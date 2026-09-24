<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { tiptapToText, type TipTapDoc } from '$lib/tiptap/serialize';
	import { BookOpen, Send } from 'lucide-svelte';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import type { PageData } from './$types';
	import type { WikiCategory } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const canCreate = $derived(data.authed && data.userPermissions?.includes('wiki.edit'));

	let title = $state('');
	let slug = $state('');
	let category = $state<WikiCategory | ''>('');
	let summary = $state('');
	let bodyHtml = $state('');
	let bodyJson = $state<unknown>(null);
	let categories = $state<{ slug: WikiCategory; label: string }[]>([]);
	const categoryOptions = $derived(categories.map((c) => ({ value: c.slug, label: c.label })));
	let catLoading = $state(false);
	let loading = $state(false);
	let errorKey = $state<string | null>(null);
	let fieldErrors = $state<Record<string, string>>({});

	$effect(() => {
		catLoading = true;
		(async () => {
			try {
				const catRes = await api.get<{ slug: WikiCategory; label_key: string; label: string }[]>(`/wiki/categories?space=${encodeURIComponent(data.spaceSlug)}`);
				const catItems = catRes.data ?? [];
				categories = catItems.map((c: { slug: WikiCategory; label: string }) => ({ slug: c.slug, label: c.label }));
				if (categories.length > 0 && category === '') {
					category = categories[0].slug;
				}
			} catch {
				/* keep empty */
			} finally {
				catLoading = false;
			}
		})();
	});

	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^\w\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onTitleInput() {
		if (slug === '' || slug === slugify(title.slice(0, -1))) {
			slug = slugify(title);
		}
	}

	function validate(): boolean {
		fieldErrors = {};
		if (title.trim().length < 2) {
			fieldErrors.title = t('wiki.create_title_short', 'Title must be at least 2 characters.');
		}
		if (slug.length < 1 || !/^[a-z0-9-]+$/.test(slug)) {
			fieldErrors.slug = t('wiki.create_slug_invalid', 'Slug must be lowercase letters, numbers, and hyphens.');
		}
		if (category === '') {
			fieldErrors.category = t('wiki.create_category_required', 'Choose a category.');
		}
		if (tiptapToText(bodyJson as unknown as TipTapDoc | null).trim().length === 0) {
			fieldErrors.content = t('wiki.content_required', 'Content is required.');
		}
		return Object.keys(fieldErrors).length === 0;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canCreate) return;
		if (!validate()) return;

		loading = true;
		errorKey = null;
		try {
			const res = await api.post<{
				data?: { page_id: string; revision_id: string; slug: string; status: string };
			}>('/wiki/' + encodeURIComponent(data.spaceSlug), {
				title: title.trim(),
				slug,
				category,
				content: bodyJson,
				summary: summary.trim() || undefined
			});
			if (res.error) {
				if (res.status === 403 && res.error.code === 'insufficient_reputation') {
					errorKey = 'wiki.insufficient_reputation';
				} else if (res.error.code === 'page_exists') {
					errorKey = 'wiki.create_page_exists';
				} else if (res.error.code === 'duplicate_content') {
					errorKey = 'wiki.duplicate_content';
				} else if (res.error.code === 'community_edits_disabled') {
					errorKey = 'wiki.community_edits_disabled';
				} else {
					errorKey = 'wiki.create_failed';
				}
				return;
			}
			const pageSlug = res.data?.data?.slug ?? slug;
			await goto(`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(pageSlug)}`);
		} catch {
			errorKey = 'wiki.create_failed';
		} finally {
			loading = false;
		}
	}

	const titleText = $derived(`${t('wiki.create_title', 'Create page')} — ${data.spaceSlug}`);
</script>

<svelte:head>
	<title>{titleText}</title>
	<meta name="description" content={t('wiki.create_description', 'Create a new wiki page')} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl p-4 sm:p-6">
	<div class="mb-6 flex items-center gap-3">
		<a
			href="/wiki/{data.spaceSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			← {t('wiki.back_to_index', 'Back')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('wiki.create_title', 'Create page')} — {data.spaceSlug}
		</h1>
	</div>

	{#if !wikiEnabled}
		<EmptyState
			icon={BookOpen}
			title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
			body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
		/>
	{:else if !canCreate}
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
			{t('wiki.create_permission', 'You must be signed in with permission to create wiki pages.')}
		</div>
	{:else}
		<form onsubmit={submit} class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div>
				<label for="wiki-create-title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_title_label', 'Page title')}
				</label>
				<input
					id="wiki-create-title"
					type="text"
					bind:value={title}
					oninput={onTitleInput}
					placeholder={t('wiki.create_title_placeholder', 'e.g. Cost of Living')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
				{#if fieldErrors.title}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.title}</p>
				{/if}
			</div>

			<div>
				<label for="wiki-create-slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_slug_label', 'URL slug')}
				</label>
				<input
					id="wiki-create-slug"
					type="text"
					bind:value={slug}
					placeholder={t('wiki.create_slug_placeholder', 'cost-of-living')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
				{#if fieldErrors.slug}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.slug}</p>
				{/if}
			</div>

			<div>
				<label for="wiki-create-category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_category_label', 'Category')}
				</label>
				<SelectField
					id="wiki-create-category"
					value={category}
					options={categoryOptions}
					disabled={catLoading}
					onValueChange={(v) => (category = v as WikiCategory)}
				/>
				{#if fieldErrors.category}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.category}</p>
				{/if}
			</div>

			<div>
				<label for="wiki-create-summary" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.edit_summary_label', 'Edit summary')}
				</label>
				<input
					id="wiki-create-summary"
					type="text"
					bind:value={summary}
					placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>

			<div>
				<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.content_label', 'Content')}
				</span>
				<PostEditor
					html={bodyHtml}
					onedit={() => {}}
					onhtml={(v) => (bodyHtml = v)}
					onjson={(v) => (bodyJson = v)}
				/>
				{#if fieldErrors.content}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.content}</p>
				{/if}
			</div>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey, t('wiki.create_failed', 'Failed to create page. Please try again.'))}
				</p>
			{/if}

			<div class="flex justify-end gap-2 pt-1">
				<a
					href="/wiki/{data.spaceSlug}"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('wiki.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					<Send size={14} />
					{loading ? '...' : t('wiki.create_submit', 'Create page')}
				</button>
			</div>
		</form>
	{/if}
</div>
