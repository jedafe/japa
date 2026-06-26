<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		position: { top: number; left: number };
		onSelect: (item: { id: string; type: string; label: string }) => void;
		onClose: () => void;
	}

	let { position, onSelect, onClose }: Props = $props();

	let query = $state('');
	let items = $state<{ id: string; type: string; label: string }[]>([]);
	let selectedIndex = $state(0);

	$effect(() => {
		if (query) {
			items = [
				{ id: '1', type: 'content', label: 'How to Apply for a Visa' },
				{ id: '2', type: 'space', label: 'Canada' },
				{ id: '3', type: 'content', label: 'Job Search Tips' },
			].filter(i => i.label.toLowerCase().includes(query.toLowerCase()));
			selectedIndex = 0;
		} else {
			items = [];
		}
	});

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % items.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + items.length) % items.length;
		} else if (e.key === 'Enter' && items[selectedIndex]) {
			e.preventDefault();
			onSelect(items[selectedIndex]);
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
	class="interlink-popup"
	style="top: {position.top}px; left: {position.left}px"
>
	<input
		type="text"
		bind:value={query}
		placeholder="Link to content or space..."
		class="interlink-input"
	/>
	
	{#if items.length > 0}
		<div class="item-list">
			{#each items as item, i}
				<button
					class="item"
					class:selected={i === selectedIndex}
					onclick={() => onSelect(item)}
				>
					<span class="type-badge">{item.type}</span>
					<span class="label">{item.label}</span>
				</button>
			{/each}
		</div>
	{:else if query}
		<div class="no-results">No results found</div>
	{/if}
</div>

<style>
	.interlink-popup {
		position: fixed;
		z-index: 50;
		background: var(--color-background);
		border: 1px solid var(--color-muted);
		border-radius: 0.5rem;
		box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1);
		padding: 0.5rem;
		min-width: 300px;
	}
	
	.interlink-input {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid var(--color-muted);
		border-radius: 0.25rem;
		margin-bottom: 0.5rem;
		background: var(--color-background);
		color: var(--color-foreground);
	}
	
	.item-list {
		max-height: 250px;
		overflow-y: auto;
	}
	
	.item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.5rem;
		border-radius: 0.25rem;
		text-align: left;
		background: transparent;
		border: none;
		cursor: pointer;
	}
	
	.item:hover,
	.item.selected {
		background: color-mix(in srgb, var(--color-muted) 10%, transparent);
	}
	
	.type-badge {
		font-size: 0.75rem;
		padding: 0.125rem 0.375rem;
		border-radius: 0.25rem;
		background: color-mix(in srgb, var(--color-muted) 20%, transparent);
		color: var(--color-muted);
		text-transform: uppercase;
	}
	
	.label {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-foreground);
	}
	
	.no-results {
		padding: 0.5rem;
		color: var(--color-muted);
		text-align: center;
	}
</style>
