<script lang="ts">
	// AchievementReactionsEditor — editor for the system_settings.achievement_reactions
	// key (§89 Achievement Feed). The setting stores a JSONB array of
	// {emoji, label} pairs that define the curated reaction set shown on
	// achievement posts.
	//
	// Ref: japaconnect-engineering/sections/89-achievement-feed.md
	// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2, GripVertical } from 'lucide-svelte';

	interface Reaction {
		emoji: string;
		label: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	} = $props();

	let reactions = $state<Reaction[]>([]);
	let lastEmitted = '';

	$effect(() => {
		const serialized = JSON.stringify(value ?? '[]');
		if (serialized === lastEmitted) return;
		reactions = parseReactions(value);
	});

	function parseReactions(v: string): Reaction[] {
		if (!v) return [];
		try {
			const arr = JSON.parse(v);
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(r): r is Record<string, string> =>
						typeof r === 'object' && r !== null && typeof r.emoji === 'string'
				)
				.map((r) => ({
					emoji: r.emoji,
					label: typeof r.label === 'string' ? r.label : ''
				}));
		} catch {
			return [];
		}
	}

	function emit(next: Reaction[]) {
		reactions = next;
		const serialized = JSON.stringify(next);
		lastEmitted = serialized;
		onchange(serialized);
	}

	function updateField(index: number, field: keyof Reaction, val: string) {
		const next = reactions.map((r, i) => (i === index ? { ...r, [field]: val } : r));
		emit(next);
	}

	function removeReaction(index: number) {
		emit(reactions.filter((_, i) => i !== index));
	}

	function moveReaction(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= reactions.length) return;
		const next = [...reactions];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}

	function addReaction() {
		emit([...reactions, { emoji: '🎉', label: '' }]);
	}
</script>

<div class="space-y-2">
	{#if reactions.length === 0}
		<p class="text-sm text-muted-foreground">
			{t('settings.achievement_reactions_empty', 'No reactions configured. Add emoji + label pairs for the achievement feed.')}
		</p>
	{/if}

	{#each reactions as reaction, i (i)}
		<div class="flex items-center gap-2 rounded-lg border border-border-input bg-background p-2">
			<!-- Reorder handles -->
			<div class="flex shrink-0 flex-col">
				<button
					type="button"
					class="rounded p-0.5 text-muted-foreground disabled:opacity-30"
					disabled={disabled || i === 0}
					onclick={() => moveReaction(i, -1)}
					aria-label="Move up"
				>
					<GripVertical class="h-3.5 w-3.5 rotate-180" />
				</button>
				<button
					type="button"
					class="rounded p-0.5 text-muted-foreground disabled:opacity-30"
					disabled={disabled || i === reactions.length - 1}
					onclick={() => moveReaction(i, 1)}
					aria-label="Move down"
				>
					<GripVertical class="h-3.5 w-3.5" />
				</button>
			</div>

			<!-- Emoji input -->
			<input
				type="text"
				value={reaction.emoji}
				maxlength="2"
				class="h-9 w-12 shrink-0 rounded-md border border-border-input bg-background px-2 py-1.5 text-center text-lg"
				disabled={disabled}
				oninput={(e) => updateField(i, 'emoji', (e.target as HTMLInputElement).value)}
				aria-label="Emoji"
			/>

			<!-- Label input -->
			<input
				type="text"
				value={reaction.label}
				placeholder={t('settings.achievement_reactions_label_ph', 'Label (e.g. Celebrate)')}
				class="h-9 flex-1 rounded-md border border-border-input bg-background px-2.5 py-1.5 text-sm"
				disabled={disabled}
				oninput={(e) => updateField(i, 'label', (e.target as HTMLInputElement).value)}
				aria-label="Label"
			/>

			<!-- Preview -->
			<span class="shrink-0 text-sm text-muted-foreground">
				{reaction.emoji} {reaction.label}
			</span>

			<!-- Remove -->
			<button
				type="button"
				class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-red-500/10 hover:text-red-500 disabled:opacity-30"
				disabled={disabled}
				onclick={() => removeReaction(i)}
				aria-label="Remove reaction"
			>
				<Trash2 class="h-4 w-4" />
			</button>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-border-input px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted/50 disabled:opacity-50"
		disabled={disabled}
		onclick={addReaction}
	>
		<Plus class="h-4 w-4" />
		{t('settings.achievement_reactions_add', 'Add reaction')}
	</button>
</div>
