<script lang="ts">
	import { t } from '$lib/i18n';
	import type { PageData } from './$types';
	import SourceBlockButton from '$lib/components/source/SourceBlockButton.svelte';

	let { data }: { data: PageData } = $props();
	// svelte-ignore state_referenced_locally
	let sources = $state(data.sources);

	$effect(() => {
		sources = data.sources;
	});

	function removeSource(slug: string) {
		sources = sources.filter((s) => s.slug !== slug);
	}
</script>

<svelte:head>
	<title>{t('sources.blocked.title', 'Blocked sources — JapaConnect')}</title>
</svelte:head>

<div class="flex flex-col gap-4 p-4">
	<div>
		<h1 class="text-xl font-bold">{t('sources.blocked.heading', 'Blocked sources')}</h1>
		<p class="text-sm text-[var(--color-text-muted)]">
			{t('sources.blocked.description', 'Sources you have blocked will no longer appear in your feeds.')}
		</p>
	</div>

	{#if sources.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.blocked.empty', 'You have not blocked any sources.')}
		</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each sources as source (source.slug)}
				<div
					class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
				>
					<div class="flex flex-col gap-1">
						<span class="font-medium">{source.name}</span>
						<span class="text-sm text-[var(--color-text-muted)]">{source.domain}</span>
					</div>
					<SourceBlockButton
						slug={source.slug}
						initial={true}
						onChange={(isBlocked) => {
							if (!isBlocked) removeSource(source.slug);
						}}
					/>
				</div>
			{/each}
		</div>
	{/if}
</div>
