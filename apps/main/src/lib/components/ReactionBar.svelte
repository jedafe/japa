<script lang="ts">
	import { Loader2 } from 'lucide-svelte';
	import { api, type ReactionType, type ContentReaction } from '$lib/api';

	let { contentId }: { contentId: string } = $props();

	let reactionTypes = $state<ReactionType[]>([]);
	let reactions = $state<ContentReaction[]>([]);
	let loading = $state(true);
	let reacting = $state<string | null>(null);
	let hoveredEmoji = $state<string | null>(null);

	$effect(() => {
		loadAll();
	});

	async function loadAll() {
		loading = true;
		try {
			const [types, existing] = await Promise.all([
				api.listReactionTypes(),
				api.getReactions(contentId)
			]);
			reactionTypes = types;
			reactions = existing;
		} catch {
			// reactions are non-critical
		} finally {
			loading = false;
		}
	}

	const groupedReactions = $derived.by(() => {
		const map = new Map<string, { type: ReactionType; count: number; userReacted: boolean; reactionId?: string }>();
		for (const rt of reactionTypes) {
			map.set(rt.key, { type: rt, count: 0, userReacted: false });
		}
		for (const r of reactions) {
			const entry = map.get(r.reaction_type.key);
			if (entry) {
				entry.count++;
				entry.userReacted = true;
				entry.reactionId = r.id;
			}
		}
		return Array.from(map.values()).filter((e) => e.count > 0 || reactionTypes.length <= 6);
	});

	async function toggleReaction(entry: { type: ReactionType; count: number; userReacted: boolean; reactionId?: string }) {
		reacting = entry.type.key;
		try {
			if (entry.userReacted && entry.reactionId) {
				await api.removeReaction(contentId, entry.type.key);
				reactions = reactions.filter((r) => r.id !== entry.reactionId);
			} else {
				const result = await api.addReaction(contentId, entry.type.key);
				reactions = [...reactions, {
					id: result.id,
					reaction_type: entry.type,
					user_id: 'me',
					created_at: new Date().toISOString()
				}];
			}
		} catch {
			// ignore reaction errors
		} finally {
			reacting = null;
		}
	}

	function usersWhoReacted(entry: { type: ReactionType; count: number }): string {
		return `${entry.count} ${entry.count === 1 ? 'person' : 'people'} reacted ${entry.type.emoji}`;
	}
</script>

{#if !loading}
	<div class="mt-4 flex flex-wrap items-center gap-2">
		{#each groupedReactions as entry (entry.type.key)}
			<div class="relative">
				<button
					onclick={() => toggleReaction(entry)}
					onmouseenter={() => (hoveredEmoji = entry.type.key)}
					onmouseleave={() => (hoveredEmoji = null)}
					disabled={reacting !== null}
					class="flex items-center gap-1 rounded-full border px-2.5 py-1 text-sm transition-all hover:scale-105 disabled:opacity-50"
					style="border-color: {entry.userReacted ? 'var(--color-primary)' : 'var(--color-border)'}; background-color: {entry.userReacted ? 'color-mix(in srgb, var(--color-primary) 8%, transparent)' : 'var(--color-background)'}"
					aria-label="{entry.type.label} reaction"
				>
					{#if reacting === entry.type.key}
						<Loader2 class="h-3.5 w-3.5 animate-spin" style="color: var(--color-muted)" />
					{:else}
						<span class="text-base">{entry.type.emoji}</span>
					{/if}
					{#if entry.count > 0}
						<span class="text-xs font-medium" style="color: var(--color-muted)">{entry.count}</span>
					{/if}
				</button>
				{#if hoveredEmoji === entry.type.key && entry.count > 0}
					<div
						class="absolute bottom-full left-1/2 z-10 mb-2 -translate-x-1/2 whitespace-nowrap rounded-lg px-3 py-1.5 text-xs shadow-lg"
						style="background-color: var(--color-foreground); color: var(--color-background)"
						role="tooltip"
					>
						{usersWhoReacted(entry)}
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
