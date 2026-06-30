<script lang="ts">
	let {
		lines = 3,
		height = '4',
		variant = 'default'
	}: {
		lines?: number;
		height?: string;
		variant?: 'default' | 'card' | 'profile';
	} = $props();
</script>

{#if variant === 'card'}
	<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-start gap-3">
			<div class="skeleton h-10 w-10 rounded-xl"></div>
			<div class="flex-1 space-y-2">
				<div class="skeleton h-4 w-3/4 rounded-lg"></div>
				<div class="skeleton h-3 w-1/3 rounded-lg"></div>
			</div>
		</div>
		<div class="mt-4 space-y-2">
			<div class="skeleton h-3 w-full rounded-lg"></div>
			<div class="skeleton h-3 w-5/6 rounded-lg"></div>
		</div>
		<div class="mt-4 flex gap-3">
			<div class="skeleton h-6 w-16 rounded-full"></div>
			<div class="skeleton h-6 w-20 rounded-full"></div>
		</div>
	</div>
{:else if variant === 'profile'}
	<div class="flex items-center gap-3">
		<div class="skeleton h-12 w-12 rounded-full"></div>
		<div class="flex-1 space-y-2">
			<div class="skeleton h-4 w-1/2 rounded-lg"></div>
			<div class="skeleton h-3 w-1/3 rounded-lg"></div>
		</div>
	</div>
{:else}
	<div class="space-y-3">
		{#each Array(lines) as _, i}
			<div
				class="skeleton rounded-lg"
				style="width: {i === lines - 1 ? '60%' : '100%'}; height: {height}px"
			></div>
		{/each}
	</div>
{/if}
