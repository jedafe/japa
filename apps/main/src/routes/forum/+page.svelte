<script lang="ts">
	import { MessageSquare, Plus, CheckCircle2, Tag, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, getToken, type ForumThread } from '$lib/api';
	import { onMount } from 'svelte';

	let threads = $state<ForumThread[]>([]);
	let loading = $state(true);
	let filter = $state<'all' | 'solved' | 'unsolved'>('all');
	let selectedTag = $state('');
	let availableTags = $state<Array<{ id: string; name: string; slug: string; thread_count: number }>>([]);
	let currentPage = $state(0);
	let totalPages = $state(1);

	const filterOptions = [
		{ value: 'all' as const, label: 'All Threads' },
		{ value: 'unsolved' as const, label: 'Open' },
		{ value: 'solved' as const, label: 'Solved' }
	];

	async function loadThreads() {
		loading = true;
		try {
			const params: Record<string, unknown> = {
				limit: 20,
				offset: currentPage * 20
			};
			if (filter === 'solved') params.solved = true;
			else if (filter === 'unsolved') params.solved = false;
			if (selectedTag) params.tag = selectedTag;

			const result = await api.getForumThreads(params as Parameters<typeof api.getForumThreads>[0]);
			threads = result.threads;
			totalPages = result.total_pages;
		} catch {
			threads = [];
		} finally {
			loading = false;
		}
	}

	async function loadTags() {
		try {
			availableTags = await api.getForumTags();
		} catch {
			availableTags = [];
		}
	}

	function setFilter(f: 'all' | 'solved' | 'unsolved') {
		filter = f;
		currentPage = 0;
		loadThreads();
	}

	function setTag(tag: string) {
		selectedTag = selectedTag === tag ? '' : tag;
		currentPage = 0;
		loadThreads();
	}

	function nextPage() {
		if (currentPage < totalPages - 1) {
			currentPage++;
			loadThreads();
		}
	}

	function prevPage() {
		if (currentPage > 0) {
			currentPage--;
			loadThreads();
		}
	}

	function timeAgo(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		return `${days}d ago`;
	}

	onMount(() => {
		loadThreads();
		loadTags();
	});
</script>

<svelte:head>
	<title>Forum | JapaConnect</title>
	<meta name="description" content="Ask questions, share experiences, and help fellow Nigerians navigate life abroad." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Forum</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Ask questions, share experiences, and help others.
			</p>
		</div>
		<a
			href="/forum/new"
			class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			<Plus class="h-4 w-4" />
			New Thread
		</a>
	</header>

	<!-- Filters -->
	<div class="mb-4 flex gap-2 overflow-x-auto pb-2">
		{#each filterOptions as opt}
			<button
				onclick={() => setFilter(opt.value)}
				class="shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="background-color: {filter === opt.value ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {filter === opt.value ? 'white' : 'var(--color-foreground)'}; border: 1px solid {filter === opt.value ? 'transparent' : 'var(--color-border)'}"
			>
				{opt.label}
			</button>
		{/each}
	</div>

	<!-- Tags -->
	{#if availableTags.length > 0}
		<div class="mb-6 flex flex-wrap gap-1.5">
			{#each availableTags as tag}
				<button
					onclick={() => setTag(tag.slug)}
					class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors"
					style="background-color: {selectedTag === tag.slug ? 'var(--color-primary)' : 'var(--color-background)'}; color: {selectedTag === tag.slug ? 'white' : 'var(--color-muted)'}; border: 1px solid {selectedTag === tag.slug ? 'transparent' : 'var(--color-border)'}"
				>
					<Tag class="h-3 w-3" />
					{tag.name}
					<span class="opacity-60">({tag.thread_count})</span>
				</button>
			{/each}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div
					class="h-24 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if threads.length === 0}
		<EmptyState
			icon={MessageSquare}
			title="No threads yet"
			message="Be the first to start a conversation in the forum."
		/>
	{:else}
		<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)">
			{#each threads as thread (thread.id)}
				<a
					href="/forum/{thread.slug}"
					class="flex items-start gap-4 p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
				>
					<!-- Vote score -->
					<div class="flex flex-col items-center gap-0.5 pt-0.5">
						<span class="text-sm font-semibold" style="color: var(--color-foreground)">
							{thread.score}
						</span>
						<span class="text-[10px] uppercase tracking-wide" style="color: var(--color-muted)">votes</span>
					</div>

					<!-- Reply count -->
					<div class="flex flex-col items-center gap-0.5 pt-0.5">
						<span class="text-sm font-semibold" style="color: var(--color-foreground)">
							{thread.reply_count}
						</span>
						<span class="text-[10px] uppercase tracking-wide" style="color: var(--color-muted)">replies</span>
					</div>

					<!-- Content -->
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<h3
								class="text-sm font-semibold leading-snug"
								style="color: var(--color-foreground)"
							>
								{thread.title}
							</h3>
							{#if thread.solved}
								<CheckCircle2 class="h-4 w-4 shrink-0" style="color: var(--color-success, #16a34a)" />
							{/if}
						</div>
						<div class="mt-1.5 flex flex-wrap items-center gap-2 text-xs" style="color: var(--color-muted)">
							<span>{thread.author_name}</span>
							<span>&middot;</span>
							<span>{timeAgo(thread.created_at)}</span>
							{#if thread.space_name}
								<span>&middot;</span>
								<span class="rounded-full px-2 py-0.5" style="background-color: var(--color-background)">
									{thread.space_name}
								</span>
							{/if}
						</div>
						{#if thread.tags.length > 0}
							<div class="mt-2 flex flex-wrap gap-1">
								{#each thread.tags as tag}
									<span
										class="rounded-full px-2 py-0.5 text-[11px]"
										style="background-color: var(--color-background); color: var(--color-muted)"
									>
										{tag}
									</span>
								{/each}
							</div>
						{/if}
					</div>
				</a>
			{/each}
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<div class="mt-6 flex items-center justify-between">
				<button
					onclick={prevPage}
					disabled={currentPage === 0}
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-40"
					style="color: var(--color-foreground); background-color: var(--color-surface); border: 1px solid var(--color-border)"
				>
					Previous
				</button>
				<span class="text-sm" style="color: var(--color-muted)">
					Page {currentPage + 1} of {totalPages}
				</span>
				<button
					onclick={nextPage}
					disabled={currentPage >= totalPages - 1}
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-40"
					style="color: var(--color-foreground); background-color: var(--color-surface); border: 1px solid var(--color-border)"
				>
					Next
				</button>
			</div>
		{/if}
	{/if}
</main>
