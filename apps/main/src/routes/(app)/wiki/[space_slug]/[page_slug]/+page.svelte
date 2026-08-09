// Ref: japaconnect-engineering/sections/80-destination-wiki.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { BookOpen, Lock, Pencil, History, ArrowLeft, Send } from 'lucide-svelte';
	import type { WikiCategory, WikiPage } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function categoryLabel(cat: WikiCategory): string {
		return t(`wiki.category_${cat}`, cat.replace(/_/g, ' '));
	}

	// Render content: if it's an HTML string, render directly. If it's TipTap
	// JSON (object), produce a plain-text approximation by walking the tree.
	function renderContent(content: WikiPage['current_content']): string {
		if (content == null) return '';
		if (typeof content === 'string') return content;
		if (typeof content === 'object') {
			return tiptapToText(content as Record<string, unknown>);
		}
		return '';
	}

	function tiptapToText(node: Record<string, unknown>): string {
		const type = node.type;
		let out = '';
		const content = node.content;
		if (Array.isArray(content)) {
			for (const child of content) {
				if (child && typeof child === 'object') {
					out += tiptapToText(child as Record<string, unknown>);
				}
			}
		}
		const text = node.text;
		if (typeof text === 'string') out += text;
		// Add line breaks after block nodes.
		if (type === 'paragraph' || type === 'heading' || type === 'codeBlock') {
			out += '\n';
		}
		return out;
	}

	const htmlContent = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return '';
		if (typeof c === 'string') return c;
		// Object/TipTap JSON — render as preformatted plain text approximation.
		return '';
	});

	const textContent = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return '';
		if (typeof c === 'object') return renderContent(c);
		return '';
	});

	const isHtml = $derived(typeof data.page?.current_content === 'string');

	// Edit form state.
	let editing = $state(false);
	let contentInput = $state('');
	let summary = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let errorKey = $state('');

	function startEdit() {
		const c = data.page?.current_content;
		if (typeof c === 'string') {
			contentInput = c;
		} else if (c && typeof c === 'object') {
			contentInput = renderContent(c);
		} else {
			contentInput = '';
		}
		summary = '';
		submitted = false;
		errorKey = '';
		editing = true;
	}

	function cancelEdit() {
		editing = false;
	}

	async function submitRevision(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!data.page) return;
		if (!contentInput.trim()) {
			errorKey = 'wiki.content_required';
			return;
		}
		if (!summary.trim()) {
			errorKey = 'wiki.summary_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string; status: string }>(
				`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}/revisions`,
				{ content: contentInput, summary }
			);
			if (res.data) {
				submitted = true;
				editing = false;
			} else {
				errorKey = 'wiki.submit_failed';
			}
		} catch {
			errorKey = 'wiki.submit_failed';
		} finally {
			loading = false;
		}
	}

	const title = $derived(
		data.page ? `${data.page.title} — ${t('wiki.page_title_suffix')}` : t('wiki.page_title_suffix')
	);
	const description = $derived(t('wiki.page_description'));
	const canEdit = $derived(!!data.page && !data.page.is_locked);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/wiki/{data.spaceSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('wiki.back_to_index', 'Back to wiki')}
		</a>
	</div>

	<div class="p-4">
		{#if data.loadError || !data.page}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.page_not_found', 'Wiki page not found')}
				body={t('wiki.page_not_found_body', "The wiki page you're looking for doesn't exist or has been removed.")}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if submitted}
			<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
				<p class="text-lg font-medium text-green-600">
					{t('wiki.revision_submitted', 'Your revision has been submitted for review.')}
				</p>
				<div class="mt-4 flex justify-center gap-3">
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}"
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.back_to_index', 'Back to page')}
					</a>
					<button
						type="button"
						onclick={() => (submitted = false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.edit', 'Edit again')}
					</button>
				</div>
			</div>
		{:else}
			<article class="mx-auto max-w-3xl">
				<header class="mb-4 flex flex-wrap items-center gap-2">
					<h1 class="text-2xl font-semibold text-[var(--color-text)]">{data.page.title}</h1>
					<span class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
						{categoryLabel(data.page.category as WikiCategory)}
					</span>
					{#if data.page.is_locked}
						<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-600">
							<Lock size={12} /> {t('wiki.locked', 'Locked')}
						</span>
					{/if}
				</header>

				<div class="mb-6 flex flex-wrap items-center gap-3 text-sm">
					{#if canEdit}
						<button
							type="button"
							onclick={startEdit}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							<Pencil size={14} /> {t('wiki.edit', 'Edit')}
						</button>
					{:else if data.page.is_locked}
						<span class="text-xs text-[var(--color-muted)]">{t('wiki.locked_body', 'This page is locked and cannot be edited.')}</span>
					{/if}
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}/history"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<History size={14} /> {t('wiki.history', 'History')}
					</a>
				</div>

				{#if editing}
					<form onsubmit={submitRevision} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
						<div class="flex flex-col gap-1.5">
							<label for="wiki-content" class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.content_label', 'Content')}
							</label>
							<textarea
								id="wiki-content"
								rows={12}
								value={contentInput}
								oninput={(e) => (contentInput = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
							></textarea>
						</div>
						<div class="flex flex-col gap-1.5">
							<label for="wiki-summary" class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.edit_summary_label', 'Edit summary')}
							</label>
							<input
								id="wiki-summary"
								type="text"
								value={summary}
								placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
								oninput={(e) => (summary = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
							/>
						</div>

						{#if errorKey}
							<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
								{t(errorKey)}
							</p>
						{/if}

						<div class="flex justify-end gap-2 pt-1">
							<button
								type="button"
								onclick={cancelEdit}
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('wiki.cancel', 'Cancel')}
							</button>
							<button
								type="submit"
								disabled={loading}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
							>
								<Send size={14} />
								{loading ? '...' : t('wiki.submit_revision', 'Submit revision')}
							</button>
						</div>
					</form>
				{:else}
					<div class="prose-wiki max-w-none text-sm leading-relaxed text-[var(--color-text)]">
						{#if isHtml}
							{@html htmlContent}
						{:else}
							<pre class="whitespace-pre-wrap font-sans text-sm text-[var(--color-text)]">{textContent}</pre>
						{/if}
					</div>
				{/if}
			</article>
		{/if}
	</div>
</div>
