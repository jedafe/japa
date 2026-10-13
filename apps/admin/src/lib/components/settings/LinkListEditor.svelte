<script lang="ts">
	// Repeater editor for docs-portal link settings.
	//   link-list  → [{label, url}]                              (help/contribute cards)
	//   link-cards → [{label, url, description?, icon?}]         (developer resources)
	// Emitted objects keep alphabetical key order so the settings page's
	// JSON.stringify dirty check matches the API's sorted-key serialization.
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-svelte';

	export interface LinkItem {
		description?: string;
		icon?: string;
		label: string;
		url: string;
	}

	const ICON_OPTIONS = ['play', 'key', 'file', 'code', 'book', 'terminal', 'link'] as const;

	let {
		value,
		disabled = false,
		onchange,
		maxItems = 3,
		showDescription = false,
		showIcon = false,
		addLabel = '',
		emptyHint = ''
	}: {
		value: unknown;
		disabled?: boolean;
		onchange: (next: LinkItem[]) => void;
		maxItems?: number;
		showDescription?: boolean;
		showIcon?: boolean;
		addLabel?: string;
		emptyHint?: string;
	} = $props();

	// Parse the incoming value (JSONB array or raw JSON string) into rows.
	// Re-parse only when the upstream value differs from what we last emitted,
	// so typing doesn't reset on every keystroke.
	let items = $state<LinkItem[]>([]);
	let lastEmitted = '';
	$effect(() => {
		const serialized = JSON.stringify(value ?? null);
		if (serialized === lastEmitted) return;
		items = parseItems(value);
	});

	function parseItems(v: unknown): LinkItem[] {
		if (!v) return [];
		try {
			const arr = typeof v === 'string' ? JSON.parse(v) : v;
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(e): e is Record<string, unknown> =>
						typeof e === 'object' && e !== null && typeof (e as Record<string, unknown>).label === 'string'
				)
				.map((e) => ({
					description: typeof e.description === 'string' ? e.description : '',
					icon: typeof e.icon === 'string' ? e.icon : 'link',
					label: e.label as string,
					url: typeof e.url === 'string' ? e.url : ''
				}));
		} catch {
			return [];
		}
	}

	// Emit omits empty optional fields so the stored shape stays tight, and
	// inserts keys alphabetically (description < icon < label < url) so the
	// settings page's JSON.stringify dirty check matches jsonb key sorting.
	function emit(next: LinkItem[]) {
		items = next;
		const out: LinkItem[] = next.map((it) => {
			const item: Record<string, string | undefined> = {};
			if (showDescription && it.description?.trim()) item.description = it.description.trim();
			if (showIcon && it.icon && it.icon !== 'link') item.icon = it.icon;
			item.label = it.label;
			item.url = it.url.trim();
			return item as unknown as LinkItem;
		});
		lastEmitted = JSON.stringify(out);
		onchange(out);
	}

	function setField(index: number, field: keyof LinkItem, val: string) {
		emit(items.map((it, i) => (i === index ? { ...it, [field]: val } : it)));
	}

	function addItem() {
		emit([...items, { label: '', url: '', description: '', icon: 'link' }]);
	}

	function removeItem(index: number) {
		emit(items.filter((_, i) => i !== index));
	}

	function moveItem(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= items.length) return;
		const next = [...items];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}
</script>

<div class="w-full space-y-3">
	{#if items.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">
			{emptyHint || t('settings.link_list_empty', 'No links configured.')}
		</p>
	{/if}

	{#each items as it, i (i)}
		<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="flex flex-wrap items-center gap-2">
				<span class="text-xs font-medium" style="color: var(--color-muted)">{i + 1}</span>
				<input
					type="text"
					value={it.label}
					placeholder={t('settings.link_label_ph', 'Label')}
					class="h-9 w-40 rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'label', (e.target as HTMLInputElement).value)}
				/>
				<input
					type="text"
					value={it.url}
					placeholder={t('settings.link_url_ph', 'URL (e.g. /docs/… or https://…)')}
					class="h-9 min-w-0 flex-1 rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'url', (e.target as HTMLInputElement).value)}
				/>
				{#if showIcon}
					<select
						value={it.icon ?? 'link'}
						class="h-9 w-28 rounded-md border px-2 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
						disabled={disabled}
						onchange={(e) => setField(i, 'icon', (e.target as HTMLSelectElement).value)}
					>
						{#each ICON_OPTIONS as opt (opt)}
							<option value={opt}>{opt}</option>
						{/each}
					</select>
				{/if}
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === 0}
					onclick={() => moveItem(i, -1)}
					aria-label="Move up"
				>
					<ChevronUp class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === items.length - 1}
					onclick={() => moveItem(i, 1)}
					aria-label="Move down"
				>
					<ChevronDown class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 transition-colors hover:bg-red-500/10 disabled:opacity-30"
					style="color: var(--color-danger, #ef4444)"
					disabled={disabled}
					onclick={() => removeItem(i)}
					aria-label="Remove link"
				>
					<Trash2 class="h-4 w-4" />
				</button>
			</div>
			{#if showDescription}
				<input
					type="text"
					value={it.description}
					placeholder={t('settings.link_desc_ph', 'Short description (optional)')}
					class="mt-2 h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'description', (e.target as HTMLInputElement).value)}
				/>
			{/if}
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-50"
		style="border-color: var(--color-border); color: var(--color-primary)"
		disabled={disabled || items.length >= maxItems}
		onclick={addItem}
	>
		<Plus class="h-4 w-4" />
		{addLabel || t('settings.link_list_add', 'Add link')}
	</button>
</div>
