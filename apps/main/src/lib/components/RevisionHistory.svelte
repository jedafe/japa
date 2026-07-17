<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { History, ChevronDown, ChevronUp, Loader2 } from 'lucide-svelte';
	import { api, type ContentRevision } from '$lib/api';

	let { contentId }: { contentId: string } = $props();

	let revisions = $state<ContentRevision[]>([]);
	let loading = $state(true);
	let error = $state('');
	let expanded = $state(false);
	let expandedId = $state<string | null>(null);

	$effect(() => {
		loadRevisions();
	});

	async function loadRevisions() {
		loading = true;
		error = '';
		try {
			revisions = await api.getRevisions(contentId);
		} catch {
			error = 'Failed to load revision history.';
		} finally {
			loading = false;
		}
	}

	function formatDate(dateStr: string): string {
		const d = new Date(dateStr);
		return d.toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}
</script>

<section class="mt-6 rounded-xl border" style="border-color: var(--color-border); background: var(--color-surface);">
	<button
		onclick={() => (expanded = !expanded)}
		class="flex w-full items-center justify-between p-4 text-left"
	>
		<h3 class="flex items-center gap-2 text-sm font-semibold" style="color: var(--color-foreground)">
			<History class="h-4 w-4" style="color: var(--color-primary)" />
			Revision History
			{#if revisions.length > 0}
				<span class="rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-background); color: var(--color-muted)">
					{revisions.length}
				</span>
			{/if}
		</h3>
		{#if expanded}
			<ChevronUp class="h-4 w-4" style="color: var(--color-muted)" />
		{:else}
			<ChevronDown class="h-4 w-4" style="color: var(--color-muted)" />
		{/if}
	</button>

	{#if expanded}
		<div class="border-t px-4 pb-4 pt-3" style="border-color: var(--color-border)">
			{#if loading}
				<div class="space-y-2">
					{#each [1, 2] as _}
						<div class="h-12 animate-pulse rounded-lg" style="background-color: var(--color-background)"></div>
					{/each}
				</div>
			{:else if error}
				<p class="text-sm" style="color: var(--color-danger)">{error}</p>
			{:else if revisions.length === 0}
				<p class="text-sm" style="color: var(--color-muted)">{t('lib.components.RevisionHistory.svelte.no_revision_history_available')}</p>
			{:else}
				<div class="space-y-2">
					{#each revisions as rev (rev.id)}
						<button
							onclick={() => (expandedId = expandedId === rev.id ? null : rev.id)}
							class="w-full rounded-lg p-3 text-left transition-colors"
							style="background-color: var(--color-background)"
						>
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-3">
									<span
										class="rounded-full px-2 py-0.5 text-xs font-medium"
										style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
									>
										v{rev.version}
									</span>
									<span class="text-sm font-medium" style="color: var(--color-foreground)">
										{rev.author_name}
									</span>
								</div>
								<time class="text-xs" datetime={rev.created_at} style="color: var(--color-muted)">
									{formatDate(rev.created_at)}
								</time>
							</div>
							{#if rev.summary && expandedId === rev.id}
								<p class="mt-2 text-sm" style="color: var(--color-muted)">{rev.summary}</p>
							{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</section>
