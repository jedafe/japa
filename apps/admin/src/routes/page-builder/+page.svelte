<script lang="ts">
	import { onMount } from 'svelte';
	import { Layout, AlertTriangle, CheckCircle2, XCircle } from 'lucide-svelte';
	import { pageBuilderApi, type BlockType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let blockTypes = $state<BlockType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	onMount(async () => {
		await loadBlockTypes();
	});

	async function loadBlockTypes() {
		loading = true;
		errorMessage = '';
		const result = await pageBuilderApi.getBlockTypes();
		if (result.ok) {
			blockTypes = Array.isArray(result.data) ? result.data : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	const groupedBlocks = $derived(
		blockTypes.reduce<Record<string, BlockType[]>>((acc, block) => {
			const cat = block.category || 'uncategorized';
			if (!acc[cat]) acc[cat] = [];
			acc[cat].push(block);
			return acc;
		}, {})
	);
</script>

<svelte:head>
	<title>Page Builder | JapaAdmin</title>
</svelte:head>

<div class="space-y-6">
	<header>
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<Layout class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
					{t('page_builder.title') || 'Page Builder Block Types'}
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					{t('page_builder.description') || 'Registered block types available in the page builder.'}
				</p>
			</div>
		</div>
	</header>

	{#if errorMessage}
		<div
			class="flex items-center gap-2 rounded-lg border p-3 text-sm"
			style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
		>
			<AlertTriangle class="h-4 w-4 flex-shrink-0" />
			{errorMessage}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div
					class="h-16 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if blockTypes.length === 0}
		<div
			class="rounded-xl border p-8 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<Layout class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('page_builder.empty') || 'No block types registered.'}
			</p>
		</div>
	{:else}
		{#each Object.entries(groupedBlocks) as [category, blocks]}
			<section>
				<h2
					class="mb-3 text-sm font-semibold uppercase tracking-wide"
					style="color: var(--color-muted)"
				>
					{category}
				</h2>
				<div
					class="overflow-hidden rounded-xl border"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<table class="w-full text-left text-sm">
						<thead>
							<tr style="border-bottom: 1px solid var(--color-border)">
								<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">
									{t('page_builder.name') || 'Name'}
								</th>
								<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">
									{t('page_builder.description_col') || 'Description'}
								</th>
								<th class="px-4 py-3 font-medium text-center" style="color: var(--color-muted)">
									{t('page_builder.enabled') || 'Enabled'}
								</th>
							</tr>
						</thead>
						<tbody>
							{#each blocks as block (block.name)}
								<tr style="border-bottom: 1px solid var(--color-border)">
									<td class="px-4 py-3">
										<span class="font-medium" style="color: var(--color-foreground)">
											{block.name}
										</span>
									</td>
									<td class="px-4 py-3" style="color: var(--color-muted)">
										{block.description || '—'}
									</td>
									<td class="px-4 py-3 text-center">
										{#if block.is_enabled}
											<CheckCircle2 class="inline h-4 w-4" style="color: #16a34a" />
										{:else}
											<XCircle class="inline h-4 w-4" style="color: var(--color-muted)" />
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/each}
	{/if}
</div>
