<script lang="ts">
	import { ArrowLeft, ArrowRight, Users } from 'lucide-svelte';

	let { stepData, onnext, onback } = $props();

	let recommendations = $state<Array<{ type: string; id: string; name: string; slug: string; reason: string; score: number }>>([]);
	let followedIds = $state<Set<string>>(new Set());
	let loading = $state(true);

	$effect(() => {
		loadRecommendations();
	});

	async function loadRecommendations() {
		try {
			const res = await fetch('/api/v1/onboarding/recommendations');
			if (res.ok) {
				const body = await res.json();
				recommendations = body.data || [];
			}
		} catch {
			// Recommendations are optional; show skip button on failure.
		} finally {
			loading = false;
		}
	}

	function toggleFollow(id: string) {
		if (followedIds.has(id)) {
			followedIds.delete(id);
			followedIds = new Set(followedIds);
		} else {
			followedIds = new Set([...followedIds, id]);
		}
	}

	function getReasonLabel(reason: string): string {
		switch (reason) {
			case 'geo_match': return 'Near you';
			case 'tag_overlap': return 'Matches your interests';
			case 'reputation': return 'Popular community';
			default: return 'Recommended';
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background: var(--color-primary); color: white;">
			<Users size={20} />
		</div>
		<div>
			<h2 class="text-xl font-semibold">Recommended for you</h2>
			<p class="text-sm" style="color: var(--color-muted)">Join spaces and follow people to get started</p>
		</div>
	</div>

	{#if loading}
		<div class="py-8 text-center">
			<p class="text-sm" style="color: var(--color-muted)">Loading recommendations...</p>
		</div>
	{:else if recommendations.length === 0}
		<div class="py-8 text-center">
			<p class="text-sm" style="color: var(--color-muted)">No recommendations available yet.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each recommendations as rec (rec.id)}
				<div
					class="flex items-center justify-between rounded-lg border p-4 transition-all"
					style="border-color: var(--color-border); background: var(--color-background);"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-semibold"
							style="background: var(--color-surface); color: var(--color-primary);"
						>
							{rec.name.charAt(0).toUpperCase()}
						</div>
						<div>
							<p class="text-sm font-medium">{rec.name}</p>
							<p class="text-xs" style="color: var(--color-muted)">{getReasonLabel(rec.reason)}</p>
						</div>
					</div>
					<button
						class="rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
						style={followedIds.has(rec.id)
							? 'background: var(--color-success); color: white;'
							: 'border: 1px solid var(--color-border); color: var(--color-foreground);'}
						onclick={() => toggleFollow(rec.id)}
					>
						{followedIds.has(rec.id) ? 'Following' : 'Follow'}
					</button>
				</div>
			{/each}
		</div>
	{/if}

	<div class="flex gap-3 mt-6">
		<button
			class="flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-foreground)"
			onclick={onback}
		>
			<ArrowLeft size={16} />
			Back
		</button>
		<button
			class="flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90"
			style="background: var(--color-primary)"
			onclick={onnext}
		>
			{recommendations.length === 0 ? 'Skip' : 'Continue'}
			<ArrowRight size={16} />
		</button>
	</div>
</div>
