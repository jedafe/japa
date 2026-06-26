<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		position: { top: number; left: number };
		onSelect: (tag: { id: string; label: string }) => void;
		onClose: () => void;
	}

	let { position, onSelect, onClose }: Props = $props();

	let query = $state('');
	let tags = $state<{ id: string; label: string }[]>([]);
	let selectedIndex = $state(0);

	$effect(() => {
		if (query) {
			tags = [
				{ id: '1', label: 'japa' },
				{ id: '2', label: 'immigration' },
				{ id: '3', label: 'career' },
			].filter(t => t.label.toLowerCase().includes(query.toLowerCase()));
			selectedIndex = 0;
		} else {
			tags = [];
		}
	});

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % tags.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + tags.length) % tags.length;
		} else if (e.key === 'Enter' && tags[selectedIndex]) {
			e.preventDefault();
			onSelect(tags[selectedIndex]);
		} else if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		}
	}

	onMount(() => {
		document.addEventListener('keydown', handleKeyDown);
	});

	onDestroy(() => {
		document.removeEventListener('keydown', handleKeyDown);
	});
</script>

<div
	class="tag-popup"
	style="top: {position.top}px; left: {position.left}px"
>
	<input
		type="text"
		bind:value={query}
		placeholder="Search tags..."
		class="tag-input"
	/>
	
	{#if tags.length > 0}
		<div class="tag-list">
			{#each tags as tag, i}
				<button
					class="tag-item"
					class:selected={i === selectedIndex}
					onclick={() => onSelect(tag)}
				>
					#{tag.label}
				</button>
			{/each}
		</div>
	{:else if query}
		<div class="no-results">No tags found</div>
	{/if}
</div>

<style>
	.tag-popup {
		position: fixed;
		z-index: 50;
		background: var(--color-background);
		border: 1px solid var(--color-muted);
		border-radius: 0.5rem;
		box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1);
		padding: 0.5rem;
		min-width: 200px;
	}
	
	.tag-input {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid var(--color-muted);
		border-radius: 0.25rem;
		margin-bottom: 0.5rem;
		background: var(--color-background);
		color: var(--color-foreground);
	}
	
	.tag-list {
		max-height: 200px;
		overflow-y: auto;
	}
	
	.tag-item {
		display: block;
		width: 100%;
		padding: 0.5rem;
		border-radius: 0.25rem;
		text-align: left;
		background: transparent;
		border: none;
		cursor: pointer;
		color: var(--color-foreground);
	}
	
	.tag-item:hover,
	.tag-item.selected {
		background: color-mix(in srgb, var(--color-muted) 10%, transparent);
	}
	
	.no-results {
		padding: 0.5rem;
		color: var(--color-muted);
		text-align: center;
	}
</style>
