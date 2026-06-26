<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		position: { top: number; left: number };
		onSelect: (user: { id: string; label: string }) => void;
		onClose: () => void;
	}

	let { position, onSelect, onClose }: Props = $props();

	let query = $state('');
	let users = $state<{ id: string; label: string }[]>([]);
	let selectedIndex = $state(0);

	$effect(() => {
		if (query) {
			users = [
				{ id: '1', label: 'Alice' },
				{ id: '2', label: 'Bob' },
				{ id: '3', label: 'Charlie' },
			].filter(u => u.label.toLowerCase().includes(query.toLowerCase()));
			selectedIndex = 0;
		} else {
			users = [];
		}
	});

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % users.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + users.length) % users.length;
		} else if (e.key === 'Enter' && users[selectedIndex]) {
			e.preventDefault();
			onSelect(users[selectedIndex]);
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
	class="mention-popup"
	style="top: {position.top}px; left: {position.left}px"
>
	<input
		type="text"
		bind:value={query}
		placeholder="Search users..."
		class="mention-input"
	/>
	
	{#if users.length > 0}
		<div class="user-list">
			{#each users as user, i}
				<button
					class="user-item"
					class:selected={i === selectedIndex}
					onclick={() => onSelect(user)}
				>
					{user.label}
				</button>
			{/each}
		</div>
	{:else if query}
		<div class="no-results">No users found</div>
	{/if}
</div>

<style>
	.mention-popup {
		position: fixed;
		z-index: 50;
		background: var(--color-background);
		border: 1px solid var(--color-muted);
		border-radius: 0.5rem;
		box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1);
		padding: 0.5rem;
		min-width: 250px;
	}
	
	.mention-input {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid var(--color-muted);
		border-radius: 0.25rem;
		margin-bottom: 0.5rem;
		background: var(--color-background);
		color: var(--color-foreground);
	}
	
	.user-list {
		max-height: 200px;
		overflow-y: auto;
	}
	
	.user-item {
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
	
	.user-item:hover,
	.user-item.selected {
		background: color-mix(in srgb, var(--color-muted) 10%, transparent);
	}
	
	.no-results {
		padding: 0.5rem;
		color: var(--color-muted);
		text-align: center;
	}
</style>
