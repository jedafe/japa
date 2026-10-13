<script lang="ts">
	// Editor for the docs.versions setting — the version dropdown entries in
	// the docs portal header. Serializes to [{label, url?}]; the first entry
	// is shown as the current version on the trigger button.
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-svelte';

	interface DocsVersion {
		label: string;
		url?: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: unknown;
		disabled?: boolean;
		onchange: (next: DocsVersion[]) => void;
	} = $props();

	// Parse the incoming value (JSONB array or raw JSON string) into rows.
	// Re-parse only when the upstream value differs from what we last emitted,
	// so typing doesn't reset on every keystroke.
	let versions = $state<DocsVersion[]>([]);
	let lastEmitted = '';
	$effect(() => {
		const serialized = JSON.stringify(value ?? null);
		if (serialized === lastEmitted) return;
		versions = parseVersions(value);
	});

	function parseVersions(v: unknown): DocsVersion[] {
		if (!v) return [];
		try {
			const arr = typeof v === 'string' ? JSON.parse(v) : v;
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(e): e is Record<string, unknown> =>
						(typeof e === 'object' && e !== null && typeof (e as Record<string, unknown>).label === 'string') ||
						typeof e === 'string'
				)
				.map((e) =>
					typeof e === 'string'
						? { label: e, url: '' }
						: { label: e.label as string, url: typeof e.url === 'string' ? e.url : '' }
				);
		} catch {
			return [];
		}
	}

	// Emit omits empty url so the stored shape stays [{label, url?}].
	function emit(next: DocsVersion[]) {
		versions = next;
		const out: DocsVersion[] = next.map((v) =>
			v.url?.trim() ? { label: v.label, url: v.url.trim() } : { label: v.label }
		);
		lastEmitted = JSON.stringify(out);
		onchange(out);
	}

	function setField(index: number, field: 'label' | 'url', val: string) {
		emit(versions.map((v, i) => (i === index ? { ...v, [field]: val } : v)));
	}

	function addVersion() {
		emit([...versions, { label: '', url: '' }]);
	}

	function removeVersion(index: number) {
		emit(versions.filter((_, i) => i !== index));
	}

	function moveVersion(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= versions.length) return;
		const next = [...versions];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}
</script>

<div class="w-full space-y-3">
	{#if versions.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('settings.docs_versions_empty', 'No versions — the docs header version dropdown stays hidden.')}
		</p>
	{/if}

	{#each versions as v, i (i)}
		<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="flex flex-wrap items-center gap-2">
				<span class="text-xs font-medium" style="color: var(--color-muted)">{i + 1}</span>
				<input
					type="text"
					value={v.label}
					placeholder={t('settings.docs_version_label_ph', 'Label (e.g. v1.0)')}
					class="h-9 w-32 rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'label', (e.target as HTMLInputElement).value)}
				/>
				<input
					type="text"
					value={v.url}
					placeholder={t('settings.docs_version_url_ph', 'URL (optional, e.g. /docs or https://…)')}
					class="h-9 min-w-0 flex-1 rounded-md border px-2.5 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
					disabled={disabled}
					oninput={(e) => setField(i, 'url', (e.target as HTMLInputElement).value)}
				/>
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === 0}
					onclick={() => moveVersion(i, -1)}
					aria-label="Move up"
				>
					<ChevronUp class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 disabled:opacity-30"
					style="color: var(--color-muted)"
					disabled={disabled || i === versions.length - 1}
					onclick={() => moveVersion(i, 1)}
					aria-label="Move down"
				>
					<ChevronDown class="h-4 w-4" />
				</button>
				<button
					type="button"
					class="rounded p-1 transition-colors hover:bg-red-500/10 disabled:opacity-30"
					style="color: var(--color-danger, #ef4444)"
					disabled={disabled}
					onclick={() => removeVersion(i)}
					aria-label="Remove version"
				>
					<Trash2 class="h-4 w-4" />
				</button>
			</div>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-50"
		style="border-color: var(--color-border); color: var(--color-primary)"
		disabled={disabled}
		onclick={addVersion}
	>
		<Plus class="h-4 w-4" />
		{t('settings.docs_versions_add', 'Add version')}
	</button>
</div>
