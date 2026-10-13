<script lang="ts">
	// Nested repeater for docs.categories — the admin-managed category catalog.
	//   [{name, description?, icon?, subcategories: string[]}]
	// Page-level category/subcategory fields stay free-text; this catalog
	// curates the display order, card descriptions/icons, and the canonical
	// subcategory list the docs portal merges with page-derived categories.
	// Emitted objects keep alphabetical key order (description < icon < name
	// < subcategories) so the settings page's JSON.stringify dirty check
	// matches the API's sorted-key serialization.
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-svelte';

	export interface CategoryItem {
		description?: string;
		icon?: string;
		name: string;
		subcategories: string[];
	}

	// Mirrors NAMED_CATEGORY_ICONS in the docs app's category-meta.ts —
	// 'auto' emits no icon field so the portal falls back to its keyword
	// heuristic.
	const ICON_OPTIONS = [
		'auto',
		'book',
		'guides',
		'api',
		'graduation',
		'integrations',
		'platform',
		'billing',
		'changelog',
		'package',
		'file',
		'rocket',
		'shield',
		'star',
		'play',
		'key',
		'terminal',
		'link',
		'compass',
		'user'
	] as const;

	interface Row {
		name: string;
		description: string;
		icon: string;
		subs: string[];
	}

	let {
		value,
		disabled = false,
		onchange,
		maxItems = 24
	}: {
		value: unknown;
		disabled?: boolean;
		onchange: (next: CategoryItem[]) => void;
		maxItems?: number;
	} = $props();

	// Re-parse only when the upstream value differs from what we last
	// emitted, so typing doesn't reset on every keystroke.
	let rows = $state<Row[]>([]);
	let lastEmitted = '';
	$effect(() => {
		const serialized = JSON.stringify(value ?? null);
		if (serialized === lastEmitted) return;
		rows = parseRows(value);
	});

	function parseRows(v: unknown): Row[] {
		if (!v) return [];
		try {
			const arr = typeof v === 'string' ? JSON.parse(v) : v;
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(e): e is Record<string, unknown> =>
						typeof e === 'object' && e !== null && typeof (e as Record<string, unknown>).name === 'string'
				)
				.map((e) => ({
					name: e.name as string,
					description: typeof e.description === 'string' ? e.description : '',
					icon: typeof e.icon === 'string' && e.icon ? e.icon : 'auto',
					subs: Array.isArray(e.subcategories)
						? e.subcategories.filter((s): s is string => typeof s === 'string')
						: []
				}));
		} catch {
			return [];
		}
	}

	// Emit omits empty optional fields; keys are inserted alphabetically.
	function emit(next: Row[]) {
		rows = next;
		const out: CategoryItem[] = next
			.filter((r) => r.name.trim())
			.map((r) => {
				const item: Record<string, unknown> = {};
				if (r.description.trim()) item.description = r.description.trim();
				if (r.icon && r.icon !== 'auto') item.icon = r.icon;
				item.name = r.name.trim();
				item.subcategories = r.subs.map((s) => s.trim()).filter(Boolean);
				return item as unknown as CategoryItem;
			});
		lastEmitted = JSON.stringify(out);
		onchange(out);
	}

	function setField(index: number, field: 'name' | 'description' | 'icon', val: string) {
		emit(rows.map((r, i) => (i === index ? { ...r, [field]: val } : r)));
	}

	function setSub(index: number, subIndex: number, val: string) {
		emit(
			rows.map((r, i) =>
				i === index ? { ...r, subs: r.subs.map((s, si) => (si === subIndex ? val : s)) } : r
			)
		);
	}

	function addSub(index: number) {
		emit(rows.map((r, i) => (i === index ? { ...r, subs: [...r.subs, ''] } : r)));
	}

	function removeSub(index: number, subIndex: number) {
		emit(
			rows.map((r, i) =>
				i === index ? { ...r, subs: r.subs.filter((_, si) => si !== subIndex) } : r
			)
		);
	}

	function moveSub(index: number, subIndex: number, dir: -1 | 1) {
		const r = rows[index];
		const target = subIndex + dir;
		if (target < 0 || target >= r.subs.length) return;
		const subs = [...r.subs];
		[subs[subIndex], subs[target]] = [subs[target], subs[subIndex]];
		emit(rows.map((row, i) => (i === index ? { ...row, subs } : row)));
	}

	function addRow() {
		emit([...rows, { name: '', description: '', icon: 'auto', subs: [] }]);
	}

	function removeRow(index: number) {
		emit(rows.filter((_, i) => i !== index));
	}

	function moveRow(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= rows.length) return;
		const next = [...rows];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}
</script>

<div class="w-full space-y-3">
	{#if rows.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">
			{t(
				'settings.categories_empty',
				'No categories managed yet — the portal derives them from published pages until you add some.'
			)}
		</p>
	{/if}

	{#each rows as row, i (i)}
		<div
			class="rounded-lg border p-3"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="flex flex-wrap items-center gap-2">
				<span class="text-xs font-medium" style="color: var(--color-muted)">{i + 1}</span>
				<input
					type="text"
					value={row.name}
					placeholder={t('settings.category_name_ph', 'Category name')}
					class="h-9 min-w-0 flex-1 rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'name', (e.target as HTMLInputElement).value)}
				/>
				<select
					value={row.icon}
					class="h-9 w-28 shrink-0 rounded-md border px-2 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					title={t('settings.category_icon', 'Icon')}
					onchange={(e) => setField(i, 'icon', (e.target as HTMLSelectElement).value)}
				>
					{#each ICON_OPTIONS as opt (opt)}
						<option value={opt}>{opt}</option>
					{/each}
				</select>
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === 0}
					onclick={() => moveRow(i, -1)}
					aria-label="Move up"
				>
					<ChevronUp class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === rows.length - 1}
					onclick={() => moveRow(i, 1)}
					aria-label="Move down"
				>
					<ChevronDown class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 transition-colors hover:bg-red-500/10 disabled:opacity-30"
					style="color: var(--color-danger, #ef4444)"
					disabled={disabled}
					onclick={() => removeRow(i)}
					aria-label="Remove category"
				>
					<Trash2 class="h-4 w-4" />
				</button>
			</div>
			<input
				type="text"
				value={row.description}
				placeholder={t('settings.category_desc_ph', 'Short description shown on the portal cards (optional)')}
				class="mt-2 h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
				style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
				disabled={disabled}
				oninput={(e) => setField(i, 'description', (e.target as HTMLInputElement).value)}
			/>

			<!-- Subcategory nested repeater -->
			<div class="mt-2 space-y-1.5">
				{#each row.subs as sub, si (si)}
					<div class="flex flex-wrap items-center gap-2">
						<span class="w-4 text-center text-xs" style="color: var(--color-muted)">»</span>
						<input
							type="text"
							value={sub}
							placeholder={t('settings.subcategory_name_ph', 'Subcategory name')}
							class="h-8 min-w-0 flex-1 rounded-md border px-2.5 py-1 text-sm"
							style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
							disabled={disabled}
							oninput={(e) => setSub(i, si, (e.target as HTMLInputElement).value)}
						/>
						<button
							type="button"
							class="rounded p-0.5 disabled:opacity-30"
							style="color: var(--color-muted)"
							disabled={disabled || si === 0}
							onclick={() => moveSub(i, si, -1)}
							aria-label="Move subcategory up"
						>
							<ChevronUp class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							class="rounded p-0.5 disabled:opacity-30"
							style="color: var(--color-muted)"
							disabled={disabled || si === row.subs.length - 1}
							onclick={() => moveSub(i, si, 1)}
							aria-label="Move subcategory down"
						>
							<ChevronDown class="h-3.5 w-3.5" />
						</button>
						<button
							type="button"
							class="rounded p-0.5 transition-colors hover:bg-red-500/10 disabled:opacity-30"
							style="color: var(--color-danger, #ef4444)"
							disabled={disabled}
							onclick={() => removeSub(i, si)}
							aria-label="Remove subcategory"
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				{/each}
				<button
					type="button"
					class="flex items-center gap-1.5 rounded px-1 py-1 text-xs font-medium disabled:opacity-50"
					style="color: var(--color-primary)"
					disabled={disabled || !row.name.trim()}
					onclick={() => addSub(i)}
				>
					<Plus class="h-3.5 w-3.5" />
					{t('settings.add_subcategory', 'Add subcategory')}
				</button>
			</div>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-50"
		style="border-color: var(--color-border); color: var(--color-primary)"
		disabled={disabled || rows.length >= maxItems}
		onclick={addRow}
	>
		<Plus class="h-4 w-4" />
		{t('settings.add_category', 'Add category')}
	</button>
</div>
