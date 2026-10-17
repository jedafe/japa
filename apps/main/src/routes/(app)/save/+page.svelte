<script lang="ts">
	// Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase3-design.md
	// Confirmation surface for the extension's save-to-account deep link.
	import { t } from '$lib/i18n';
	import { BookmarkCheck, BookmarkX } from 'lucide-svelte';

	let { data } = $props();
</script>

<svelte:head><title>{t('save.title', 'Save — {site}')}</title></svelte:head>

<div class="mx-auto flex max-w-md flex-col items-center gap-4 px-4 py-24 text-center">
	{#if data.saved}
		<BookmarkCheck class="h-10 w-10 text-emerald-500" />
		<h1 class="text-xl font-semibold text-slate-900 dark:text-slate-100">
			{t('save.saved_title', 'Saved to your library')}
		</h1>
		<p class="text-sm text-slate-500 dark:text-slate-400">
			{t('save.saved_body', 'Find it anytime under Bookmarks in your dashboard.')}
		</p>
		<div class="mt-2 flex gap-3">
			{#if data.returnHref}
				<a
					href={data.returnHref}
					class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
				>{t('save.open_item', 'Open')}</a>
			{/if}
			<a
				href="/dashboard/bookmarks"
				class="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
			>{t('save.view_library', 'Your bookmarks')}</a>
		</div>
	{:else}
		<BookmarkX class="h-10 w-10 text-slate-400" />
		<h1 class="text-xl font-semibold text-slate-900 dark:text-slate-100">
			{t('save.failed_title', 'Could not save')}
		</h1>
		<p class="text-sm text-slate-500 dark:text-slate-400">
			{t('save.failed_body', 'This item may no longer be available, or it is not visible to your account.')}
		</p>
		{#if data.returnHref}
			<a
				href={data.returnHref}
				class="mt-2 rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
			>{t('save.open_item', 'Open')}</a>
		{/if}
	{/if}
</div>
