<script lang="ts">
	// CsvRepeater — edit a comma-separated list as a set of text inputs with
	// add/remove controls. Used for settings such as the iframe host allowlist
	// (content.sanitization.allowed_iframe_hosts).
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import { tick } from 'svelte';

	interface Props {
		value: string;
		placeholder?: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	}

	let { value = '', placeholder = 'e.g. youtube.com', disabled = false, onchange }: Props = $props();

	let items = $state<string[]>([]);
	let lastEmitted = $state('');
	let inputRefs = $state<HTMLInputElement[]>([]);

	function splitValue(v: string): string[] {
		const unquoted = v.replace(/^"(.*)"$/, '$1');
		if (!unquoted) return [];
		return unquoted
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	function joinValue(list: string[]): string {
		return list.filter((s) => s.trim() !== '').join(',');
	}

	$effect(() => {
		const current = joinValue(splitValue(value ?? ''));
		if (current === lastEmitted) return;
		items = splitValue(value ?? '');
	});

	function emit(next: string[]) {
		items = next;
		lastEmitted = joinValue(next);
		onchange(joinValue(next));
	}

	function updateItem(index: number, next: string) {
		const trimmed = next.trim();
		if (trimmed === '') {
			removeItem(index);
			return;
		}
		emit(items.map((item, i) => (i === index ? trimmed : item)));
	}

	function removeItem(index: number) {
		emit(items.filter((_, i) => i !== index));
	}

	async function addItem() {
		items = [...items, ''];
		await tick();
		inputRefs[items.length - 1]?.focus();
	}
</script>

<div class="w-full space-y-1.5">
	{#if items.length === 0}
		<p class="text-sm text-muted-foreground">
			{t('settings.csv_repeater_empty', 'No entries configured. Click Add to create one.')}
		</p>
	{/if}

	{#each items as item, i (i)}
		<div class="flex items-center gap-2">
			<input
				bind:this={inputRefs[i]}
				type="text"
				value={item}
				{placeholder}
				disabled={disabled}
				onchange={(e) => updateItem(i, (e.currentTarget as HTMLInputElement).value)}
				class="h-8 flex-1 rounded-md border border-border-input bg-background px-2 py-1 font-mono text-xs"
			/>
			<button
				type="button"
				class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-red-500/10 hover:text-red-500 disabled:opacity-30"
				disabled={disabled}
				onclick={() => removeItem(i)}
				aria-label={t('common.remove', 'Remove')}
			>
				<Trash2 class="h-3.5 w-3.5" />
			</button>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-1.5 rounded-lg border border-dashed border-border-input px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted/50 disabled:opacity-50"
		disabled={disabled}
		onclick={addItem}
	>
		<Plus class="h-3.5 w-3.5" />
		{t('settings.csv_repeater_add', 'Add host')}
	</button>
</div>
