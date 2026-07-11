<script lang="ts">
	interface TocItem {
		id: string;
		text: string;
		level: number;
	}

	interface Props {
		items?: TocItem[];
		title?: string;
		ordered?: boolean;
		smooth_scroll?: boolean;
		readOnly?: boolean;
	}

	let {
		items = [],
		title = 'Table of Contents',
		ordered = false,
		smooth_scroll = true,
		readOnly = false,
	}: Props = $props();

	function scrollTo(id: string) {
		if (!smooth_scroll) return;
		const el = document.getElementById(id);
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'start' });
		}
	}

	let wrapperClass = $derived(
		`block-toc${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if items.length === 0}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Headings will appear here once added to the page
		</div>
	{:else}
		<nav class="rounded-lg bg-gray-50 p-6 dark:bg-gray-800/50">
			{#if title}
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{title}</h2>
			{/if}
			{#if ordered}
				<ol class="space-y-2">
					{#each items as item}
						<li style="padding-left: {(item.level - 1) * 1.5}rem">
							<button
								class="text-sm text-blue-600 hover:text-blue-800 hover:underline dark:text-blue-400 dark:hover:text-blue-300"
								onclick={() => scrollTo(item.id)}
							>
								{item.text}
							</button>
						</li>
					{/each}
				</ol>
			{:else}
				<ul class="space-y-2">
					{#each items as item}
						<li style="padding-left: {(item.level - 1) * 1.5}rem">
							<button
								class="text-sm text-blue-600 hover:text-blue-800 hover:underline dark:text-blue-400 dark:hover:text-blue-300"
								onclick={() => scrollTo(item.id)}
							>
								{item.text}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</nav>
	{/if}
</div>
