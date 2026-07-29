<script lang="ts">
	import { PenSquare } from 'lucide-svelte';
	import ForumList from '$lib/components/forum/ForumList.svelte';
	import { t } from '$lib/i18n';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('forum.list_title', 'Forum — JapaConnect'));
	const description = $derived(
		t('forum.list_description', 'Discussions, questions, and answers from the Nigerian diaspora community.')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('forum.heading', 'Forum')}</h1>
		{#if data.authed}
			<a
				href="/create?type=forum"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('forum.new_discussion', 'New discussion')}
			</a>
		{/if}
	</div>

	<div class="p-3">
		<ForumList
			threads={data.threads}
			pageSize={data.pageSize}
			loadError={data.loadError}
		/>
	</div>
</div>
