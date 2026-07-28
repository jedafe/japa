<script lang="ts">
	// Editor for the appearance.auth_brand_slides setting.
	// Lets the admin pick multiple images from the media library and edit a
	// headline + subheading per slide. Serializes to the same JSON array shape
	// consumed by the main app's AuthShell: [{image, headline, subheading}].
	import { t } from '$lib/stores/i18n.svelte';
	import { mediaAdminApi } from '$lib/api';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Trash2, GripVertical, Image as ImageIcon, RefreshCw, X } from 'lucide-svelte';

	interface Slide {
		image: string;
		headline: string;
		subheading: string;
	}

	interface MediaItem {
		id: string;
		filename: string;
		mime_type: string;
		url?: string;
		status: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: unknown;
		disabled?: boolean;
		onchange: (next: Slide[]) => void;
	} = $props();

	// Parse the incoming value (JSONB array or raw JSON string) into slides.
	// Re-parse only when the upstream value differs from what we last emitted,
	// so editing headline/subheading inputs doesn't reset on every keystroke.
	let slides = $state<Slide[]>([]);
	let lastEmitted = '';
	$effect(() => {
		const serialized = JSON.stringify(value ?? null);
		if (serialized === lastEmitted) return;
		slides = parseSlides(value);
	});

	function parseSlides(v: unknown): Slide[] {
		if (!v) return [];
		try {
			const arr = typeof v === 'string' ? JSON.parse(v) : v;
			if (!Array.isArray(arr)) return [];
			return arr
				.filter(
					(s): s is Record<string, string> =>
						typeof s === 'object' && s !== null && typeof s.image === 'string'
				)
				.map((s) => ({
					image: s.image,
					headline: typeof s.headline === 'string' ? s.headline : '',
					subheading: typeof s.subheading === 'string' ? s.subheading : ''
				}));
		} catch {
			return [];
		}
	}

	function emit(next: Slide[]) {
		slides = next;
		lastEmitted = JSON.stringify(next);
		onchange(next);
	}

	// --- Media picker modal ---
	let pickerOpen = $state(false);
	let mediaItems = $state<MediaItem[]>([]);
	let mediaLoading = $state(false);
	let mediaError = $state('');
	let pickedIds = $state<Set<string>>(new Set());

	async function openPicker() {
		pickerOpen = true;
		pickedIds = new Set();
		await loadMedia();
	}

	async function loadMedia() {
		mediaLoading = true;
		mediaError = '';
		try {
			const result = await mediaAdminApi.list('?mime_type=image&limit=60');
			if (result.ok) {
				const data = result.data as MediaItem[] | { items?: MediaItem[] } | null;
				mediaItems = Array.isArray(data) ? data : (data?.items ?? []);
			} else {
				mediaError = result.error;
			}
		} catch (e) {
			mediaError = e instanceof Error ? e.message : 'Failed to load media';
		} finally {
			mediaLoading = false;
		}
	}

	function togglePick(id: string) {
		const next = new Set(pickedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		pickedIds = next;
	}

	function confirmPicks() {
		const picked = mediaItems.filter((m) => pickedIds.has(m.id) && m.url);
		if (picked.length === 0) {
			pickerOpen = false;
			return;
		}
		const next: Slide[] = [
			...slides,
			...picked.map((m) => ({
				image: m.url as string,
				headline: '',
				subheading: ''
			}))
		];
		emit(next);
		pickerOpen = false;
	}

	// --- Slide editing ---
	function updateField(index: number, field: keyof Slide, val: string) {
		const next = slides.map((s, i) => (i === index ? { ...s, [field]: val } : s));
		emit(next);
	}

	function removeSlide(index: number) {
		emit(slides.filter((_, i) => i !== index));
	}

	function moveSlide(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= slides.length) return;
		const next = [...slides];
		[next[index], next[target]] = [next[target], next[index]];
		emit(next);
	}
</script>

<div class="space-y-3">
	{#if slides.length === 0}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('settings.auth_slides_empty', 'No slides yet. Add images to build the auth brand carousel.')}
		</p>
	{/if}

	{#each slides as slide, i (slide.image + i)}
		<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="flex gap-3">
				<!-- Thumbnail -->
				<div class="flex shrink-0 flex-col items-center gap-1">
					<div class="h-20 w-28 overflow-hidden rounded-md border" style="border-color: var(--color-border)">
						<img src={slide.image} alt={slide.headline || `Slide ${i + 1}`} class="h-full w-full object-cover" />
					</div>
					<div class="flex gap-0.5">
						<button
							type="button"
							class="rounded p-1 text-xs disabled:opacity-30"
							style="color: var(--color-muted)"
							disabled={disabled || i === 0}
							onclick={() => moveSlide(i, -1)}
							aria-label="Move up"
						>
							<GripVertical class="h-3.5 w-3.5 rotate-180" />
						</button>
						<button
							type="button"
							class="rounded p-1 text-xs disabled:opacity-30"
							style="color: var(--color-muted)"
							disabled={disabled || i === slides.length - 1}
							onclick={() => moveSlide(i, 1)}
							aria-label="Move down"
						>
							<GripVertical class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>

				<!-- Text inputs -->
				<div class="flex flex-1 flex-col gap-2">
					<div class="flex items-center justify-between">
						<span class="text-xs font-medium" style="color: var(--color-muted)">Slide {i + 1}</span>
						<button
							type="button"
							class="rounded p-1 transition-colors hover:bg-red-500/10 disabled:opacity-30"
							style="color: var(--color-danger, #ef4444)"
							disabled={disabled}
							onclick={() => removeSlide(i)}
							aria-label="Remove slide"
						>
							<Trash2 class="h-4 w-4" />
						</button>
					</div>
					<input
						type="text"
						value={slide.headline}
						placeholder={t('settings.auth_slides_headline_ph', 'Headline (line 1)')}
						class="h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
						disabled={disabled}
						oninput={(e) => updateField(i, 'headline', (e.target as HTMLInputElement).value)}
					/>
					<input
						type="text"
						value={slide.subheading}
						placeholder={t('settings.auth_slides_subheading_ph', 'Subheading (line 2)')}
						class="h-9 w-full rounded-md border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-bg); color: var(--color-foreground)"
						disabled={disabled}
						oninput={(e) => updateField(i, 'subheading', (e.target as HTMLInputElement).value)}
					/>
				</div>
			</div>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-50"
		style="border-color: var(--color-border); color: var(--color-primary)"
		disabled={disabled}
		onclick={openPicker}
	>
		<Plus class="h-4 w-4" />
		{t('settings.auth_slides_add', 'Add images from media library')}
	</button>
</div>

<!-- Media picker modal -->
<Modal bind:open={pickerOpen} title={t('settings.auth_slides_picker_title', 'Select images')} size="xl">
	<div class="space-y-3">
		{#if mediaLoading}
			<div class="flex items-center justify-center py-12" style="color: var(--color-muted)">
				<RefreshCw class="h-5 w-5 animate-spin" />
			</div>
		{:else if mediaError}
			<p class="text-sm" style="color: var(--color-danger, #ef4444)">{mediaError}</p>
		{:else if mediaItems.length === 0}
			<div class="flex flex-col items-center justify-center gap-2 py-12" style="color: var(--color-muted)">
				<ImageIcon class="h-8 w-8" />
				<p class="text-sm">{t('settings.auth_slides_no_media', 'No images found. Upload images in the Media section first.')}</p>
			</div>
		{:else}
			<p class="text-xs" style="color: var(--color-muted)">
				{t('settings.auth_slides_picker_hint', 'Click images to select. You can pick multiple.')}
				{#if pickedIds.size > 0}
					— {pickedIds.size} selected
				{/if}
			</p>
			<div class="grid max-h-[60vh] grid-cols-3 gap-3 overflow-y-auto sm:grid-cols-4 md:grid-cols-5">
				{#each mediaItems as item (item.id)}
					{@const selected = pickedIds.has(item.id)}
					<button
						type="button"
						class="relative aspect-square overflow-hidden rounded-md border-2 transition-all"
						class:ring-2={selected}
						style={selected
							? 'border-color: var(--color-primary); --tw-ring-color: var(--color-primary)'
							: 'border-color: var(--color-border)'}
						onclick={() => togglePick(item.id)}
						aria-pressed={selected}
					>
						<img src={item.url || ''} alt={item.filename} class="h-full w-full object-cover" />
						{#if selected}
							<div class="absolute inset-0 bg-primary/30"></div>
							<div class="absolute right-1.5 top-1.5 grid h-6 w-6 place-items-center rounded-full bg-primary text-white">
								<X class="h-3.5 w-3.5 rotate-45" />
							</div>
						{/if}
					</button>
				{/each}
			</div>
		{/if}

		<div class="flex items-center justify-end gap-2 pt-2">
			<button
				type="button"
				class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors"
				style="border-color: var(--color-border); color: var(--color-foreground)"
				onclick={() => (pickerOpen = false)}
			>
				{t('common.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				class="rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
				style="background: var(--color-primary)"
				disabled={pickedIds.size === 0}
				onclick={confirmPicks}
			>
				{t('settings.auth_slides_add_selected', 'Add selected')}
			</button>
		</div>
	</div>
</Modal>
