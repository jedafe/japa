<script lang="ts">
	import { clsx, type ClassValue } from "clsx";

	let {
		title,
		description,
		actions = [],
		class: className
	}: {
		title: string;
		description?: string;
		actions?: Array<{
			label: string;
			icon?: any;
			onclick?: () => void;
			variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
			href?: string;
		}>;
		class?: ClassValue;
	} = $props();
</script>

<div class={clsx("mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between", className)}>
	<div>
		<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">{title}</h1>
		{#if description}
			<p class="mt-1 text-sm" style="color: var(--color-muted)">{description}</p>
		{/if}
	</div>

	{#if actions.length > 0}
		<div class="flex items-center gap-3">
			{#each actions as action}
				{#if action.href}
					<a
						href={action.href}
						class="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors focus-ring"
						style="background-color: var(--color-primary)"
					>
						{#if action.icon}
							<svelte:component this={action.icon} class="h-4 w-4" />
						{/if}
						{action.label}
					</a>
				{:else}
					<button
						type="button"
						onclick={action.onclick}
						class={clsx(
							"inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium shadow-sm transition-colors focus-ring",
							action.variant === "destructive"
								? "text-white"
								: "hover:bg-black/5 dark:hover:bg-white/10"
						)}
						style={action.variant === "destructive" ? "background-color: var(--color-danger)" : `border: 1px solid var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)`}
					>
						{#if action.icon}
							<svelte:component this={action.icon} class="h-4 w-4" />
						{/if}
						{action.label}
					</button>
				{/if}
			{/each}
		</div>
	{/if}
</div>
