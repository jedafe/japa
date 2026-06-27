<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { ChevronDown, Plus } from 'lucide-svelte';

	let {
		spaces = [],
		currentSpace = null,
		onselect = () => {}
	}: {
		spaces: Array<{ id: string; name: string; slug: string }>;
		currentSpace: { id: string; name: string; slug: string } | null;
		onselect: (space: { id: string; name: string; slug: string }) => void;
	} = $props();
</script>

{#if spaces.length > 0}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm font-medium hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-foreground)"
		>
			<span class="max-w-[120px] truncate">
				{currentSpace?.name || 'All Spaces'}
			</span>
			<ChevronDown class="h-3 w-3" style="color: var(--color-muted)" />
		</DropdownMenu.Trigger>
		<DropdownMenu.Content
			align="start"
			class="w-52 rounded-lg border p-1 shadow-lg"
			style="background-color: var(--color-background); border-color: var(--color-border)"
		>
			<DropdownMenu.Item
				class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
				onclick={() => onselect({ id: '', name: 'All Spaces', slug: '' })}
			>
				All Spaces
			</DropdownMenu.Item>
			<DropdownMenu.Separator class="my-1 h-px" style="background-color: var(--color-border)" />
			{#each spaces as space}
				<DropdownMenu.Item
					class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
					onclick={() => onselect(space)}
				>
					{space.name}
				</DropdownMenu.Item>
			{/each}
			<DropdownMenu.Separator class="my-1 h-px" style="background-color: var(--color-border)" />
			<DropdownMenu.Item
				class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
			>
				<Plus class="h-4 w-4" />
				<a href="/spaces/new">Create space</a>
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/if}
