<script lang="ts">
	// Editor for the docs.pinned_pages setting — the GET STARTED pins shown in
	// the docs portal sidebar and the "New to JapaConnect?" card.
	// Serializes to the same JSON array shape the docs app consumes:
	// [{audience, slug, label?}]. Audience and page are picked from real data
	// (docsApi.audiences / docsApi.list) so admins can't fat-finger a slug.
	import { t } from '$lib/stores/i18n.svelte';
	import { docsApi } from '$lib/api';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-svelte';

	interface Pin {
		audience: string;
		slug: string;
		label: string;
		icon: string;
	}

	/** Emitted shape — label/icon are omitted when empty. */
	interface StoredPin {
		audience: string;
		slug: string;
		label?: string;
		icon?: string;
	}

	const PIN_ICON_OPTIONS = ['file', 'book', 'rocket', 'compass', 'user', 'star'] as const;

	interface PageOption {
		value: string;
		label: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: unknown;
		disabled?: boolean;
		onchange: (next: StoredPin[]) => void;
	} = $props();

	// Parse the incoming value (JSONB array or raw JSON string) into pins.
	// Re-parse only when the upstream value differs from what we last emitted,
	// so typing in the label input doesn't reset on every keystroke.
	let pins = $state<Pin[]>([]);
	let lastEmitted = '';
	$effect(() => {
		const serialized = JSON.stringify(value ?? null);
		if (serialized === lastEmitted) return;
		// Iterate the parse result, not `pins` — reading `pins` here would make
		// the effect depend on the state it writes (infinite re-run loop).
		const parsed = parsePins(value);
		pins = parsed;
		for (const p of parsed) ensurePages(p.audience);
	});

	function parsePins(v: unknown): Pin[] {
		if (!v) return [];
		try {
			const arr = typeof v === 'string' ? JSON.parse(v) : v;
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(p): p is Record<string, unknown> =>
						typeof p === 'object' && p !== null && typeof (p as Record<string, unknown>).slug === 'string'
				)
				.map((p) => ({
					audience: typeof p.audience === 'string' && p.audience ? p.audience : 'users',
					slug: p.slug as string,
					label: typeof p.label === 'string' ? p.label : '',
					icon: typeof p.icon === 'string' && p.icon ? p.icon : 'file'
				}));
		} catch {
			return [];
		}
	}

	// Emit in alphabetical key order ({audience, icon?, label?, slug}) — the
	// settings API returns Go-marshalled (sorted-key) JSON, and the settings
	// page's dirty check is a raw JSON.stringify comparison, so matching key
	// order avoids phantom "modified" badges on identical content.
	function emit(next: Pin[]) {
		pins = next;
		const out: StoredPin[] = next.map((p) => ({
			audience: p.audience,
			...(p.icon && p.icon !== 'file' ? { icon: p.icon } : {}),
			...(p.label ? { label: p.label } : {}),
			slug: p.slug
		}));
		lastEmitted = JSON.stringify(out);
		onchange(out);
	}

	// --- Audience options (DOC-009: loaded from the API, hardcoded fallback) ---
	let audienceOptions = $state<PageOption[]>(
		['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin'].map((a) => ({
			value: a,
			label: a
		}))
	);

	$effect(() => {
		docsApi.audiences().then((res) => {
			if (res.ok && Array.isArray(res.data) && res.data.length > 0) {
				audienceOptions = res.data.map((a) => ({ value: a, label: a }));
			}
		});
	});

	// --- Page options, fetched once per audience and cached ---
	let pagesByAudience = $state<Record<string, PageOption[]>>({});
	let loadingAudiences = new Set<string>();

	async function ensurePages(audience: string) {
		if (!audience || pagesByAudience[audience] || loadingAudiences.has(audience)) return;
		loadingAudiences.add(audience);
		const res = await docsApi.list(`audience=${encodeURIComponent(audience)}`);
		loadingAudiences.delete(audience);
		if (!res.ok) return;
		const raw = res.data as any[] | { data: any[] };
		const rows = Array.isArray(raw) ? raw : (raw?.data ?? []);
		pagesByAudience = {
			...pagesByAudience,
			[audience]: rows
				.filter((p) => (p.review_status ?? p.status) === 'published')
				.map((p) => ({ value: p.slug as string, label: `${p.title} (${p.slug})` }))
		};
	}

	// Options for a pin's page select — the pin's current slug is always kept
	// selectable. Once the audience's pages have loaded, a missing slug is
	// flagged "(not published)"; while still loading it's shown bare.
	function pageOptionsFor(pin: Pin): PageOption[] {
		const loaded = pin.audience in pagesByAudience;
		const opts = pagesByAudience[pin.audience] ?? [];
		if (pin.slug && !opts.some((o) => o.value === pin.slug)) {
			const label = loaded ? `${pin.slug} (not published)` : pin.slug;
			return [...opts, { value: pin.slug, label }];
		}
		return opts;
	}

	function setAudience(index: number, audience: string) {
		ensurePages(audience);
		emit(pins.map((p, i) => (i === index ? { ...p, audience, slug: '' } : p)));
	}

	function setSlug(index: number, slug: string) {
		emit(pins.map((p, i) => (i === index ? { ...p, slug } : p)));
	}

	function setLabel(index: number, label: string) {
		emit(pins.map((p, i) => (i === index ? { ...p, label } : p)));
	}

	function setIcon(index: number, icon: string) {
		emit(pins.map((p, i) => (i === index ? { ...p, icon } : p)));
	}

	function addPin() {
		const audience = audienceOptions[0]?.value ?? 'users';
		ensurePages(audience);
		emit([...pins, { audience, slug: '', label: '', icon: 'file' }]);
	}

	function removePin(index: number) {
		emit(pins.filter((_, i) => i !== index));
	}

	function movePin(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= pins.length) return;
		const next = [...pins];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}
</script>

<div class="w-full space-y-3">
	{#if pins.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('settings.docs_pins_empty', 'No pinned pages. Pins appear under GET STARTED in the docs sidebar.')}
		</p>
	{/if}

	{#each pins as pin, i (i)}
		<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="flex items-center justify-between">
				<span class="text-xs font-medium" style="color: var(--color-muted)">Pin {i + 1}</span>
				<div class="flex items-center gap-0.5">
					<button
						type="button"
						class="rounded p-1 disabled:opacity-30"
						style="color: var(--color-muted)"
						disabled={disabled || i === 0}
						onclick={() => movePin(i, -1)}
						aria-label="Move up"
					>
						<ChevronUp class="h-4 w-4" />
					</button>
					<button
						type="button"
						class="rounded p-1 disabled:opacity-30"
						style="color: var(--color-muted)"
						disabled={disabled || i === pins.length - 1}
						onclick={() => movePin(i, 1)}
						aria-label="Move down"
					>
						<ChevronDown class="h-4 w-4" />
					</button>
					<button
						type="button"
						class="rounded p-1 transition-colors hover:bg-red-500/10 disabled:opacity-30"
						style="color: var(--color-danger, #ef4444)"
						disabled={disabled}
						onclick={() => removePin(i)}
						aria-label="Remove pin"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				</div>
			</div>

			<div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="pin-audience-{i}">
						{t('settings.docs_pin_audience', 'User group')}
					</label>
					<AutocompleteSelect
						id="pin-audience-{i}"
						value={pin.audience}
						options={audienceOptions}
						disabled={disabled}
						onValueChange={(v: string) => setAudience(i, v)}
					/>
				</div>
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="pin-page-{i}">
						{t('settings.docs_pin_page', 'Page')}
					</label>
					<AutocompleteSelect
						id="pin-page-{i}"
						value={pin.slug}
						options={pageOptionsFor(pin)}
						placeholder={t('settings.docs_pin_page_ph', 'Pick a published page…')}
						disabled={disabled}
						onValueChange={(v: string) => setSlug(i, v)}
					/>
				</div>
			</div>

			<div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="pin-label-{i}">
						{t('settings.docs_pin_label', 'Label (optional — defaults to the page title)')}
					</label>
					<input
						type="text"
						id="pin-label-{i}"
						value={pin.label}
						placeholder={t('settings.docs_pin_label_ph', 'e.g. Quick Start')}
						class="h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
						disabled={disabled}
						oninput={(e) => setLabel(i, (e.target as HTMLInputElement).value)}
					/>
				</div>
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="pin-icon-{i}">
						{t('settings.docs_pin_icon', 'Icon')}
					</label>
					<select
						id="pin-icon-{i}"
						value={pin.icon}
						class="h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
						disabled={disabled}
						onchange={(e) => setIcon(i, (e.target as HTMLSelectElement).value)}
					>
						{#each PIN_ICON_OPTIONS as opt (opt)}
							<option value={opt}>{opt}</option>
						{/each}
					</select>
				</div>
			</div>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-50"
		style="border-color: var(--color-border); color: var(--color-primary)"
		disabled={disabled}
		onclick={addPin}
	>
		<Plus class="h-4 w-4" />
		{t('settings.docs_pins_add', 'Add pinned page')}
	</button>
</div>
