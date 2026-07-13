<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';

	interface Props {
		position: { top: number; left: number };
		onSelect: (command: string) => void;
		onClose: () => void;
	}

	let { position, onSelect, onClose }: Props = $props();

	let selectedIndex = $state(0);

	/** Check if the current user has the AI content assist permission via feature flags. */
	function aiAssistEnabled(): boolean {
		const flags = page.data?.featureFlags ?? {};
		return flags['ai.content_assist_enabled'] === true;
	}

	const baseCommands = [
		{ id: 'heading1', label: 'Heading 1', icon: 'H1' },
		{ id: 'heading2', label: 'Heading 2', icon: 'H2' },
		{ id: 'heading3', label: 'Heading 3', icon: 'H3' },
		{ id: 'bulletList', label: 'Bullet List', icon: '•' },
		{ id: 'orderedList', label: 'Ordered List', icon: '1.' },
		{ id: 'blockquote', label: 'Quote', icon: '"' },
		{ id: 'codeBlock', label: 'Code Block', icon: '<>' },
		{ id: 'horizontalRule', label: 'Horizontal Rule', icon: '—' },
	];

	const aiCommands = [
		{ id: 'ai:improve', label: 'AI: Improve writing', icon: '✨' },
		{ id: 'ai:summarize', label: 'AI: Summarize', icon: '📝' },
		{ id: 'ai:translate_fr', label: 'AI: Translate to French', icon: '🌐' },
	];

	/** Combine base commands with AI commands when the feature flag is enabled. */
	const commands = [...baseCommands, ...(aiAssistEnabled() ? aiCommands : [])];

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % commands.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + commands.length) % commands.length;
		} else if (e.key === 'Enter') {
			e.preventDefault();
			onSelect(commands[selectedIndex].id);
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
	class="slash-command-menu z-50"
	style="top: {position.top}px; left: {position.left}px"
>
	{#each commands as command, i}
		<button
			class="slash-command-item"
			class:selected={i === selectedIndex}
			onclick={() => onSelect(command.id)}
		>
			<span class="icon">{command.icon}</span>
			<span class="label">{command.label}</span>
		</button>
	{/each}
</div>

<style>
	.slash-command-menu {
		position: fixed;
		background: var(--color-background);
		border: 1px solid var(--color-muted);
		border-radius: 0.5rem;
		box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1);
		padding: 0.25rem;
		min-width: 200px;
	}
	
	.slash-command-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.5rem 0.75rem;
		border-radius: 0.25rem;
		text-align: left;
		background: transparent;
		border: none;
		cursor: pointer;
		transition: background 0.1s;
	}
	
	.slash-command-item:hover,
	.slash-command-item.selected {
		background: color-mix(in srgb, var(--color-muted) 10%, transparent);
	}
	
	.icon {
		font-weight: 600;
		color: var(--color-muted);
		min-width: 2rem;
	}
	
	.label {
		color: var(--color-foreground);
	}
</style>
