<script lang="ts">
	import { Inbox } from 'lucide-svelte';
	import type { IconComponent } from './icons';

	interface Action {
		label: string;
		href?: string;
		onclick?: () => void;
	}

	interface Props {
		/** Lucide icon component; defaults to an inbox. */
		icon?: IconComponent;
		title: string;
		body?: string;
		action?: Action;
	}

	let { icon: Icon = Inbox, title, body, action }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
	<div
		class="grid h-14 w-14 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
	>
		<Icon size={26} strokeWidth={1.5} />
	</div>
	<h2 class="text-lg font-medium text-[var(--color-text)]">{title}</h2>
	{#if body}
		<p class="max-w-sm text-sm text-[var(--color-text-subtle)]">{body}</p>
	{/if}
	{#if action}
		{#if action.href}
			<a
				href={action.href}
				class="mt-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				{action.label}
			</a>
		{:else}
			<button
				type="button"
				onclick={action.onclick}
				class="mt-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				{action.label}
			</button>
		{/if}
	{/if}
</div>
