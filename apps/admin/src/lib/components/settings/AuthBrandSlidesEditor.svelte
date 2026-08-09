<script lang="ts">
	// Editor for the appearance.auth_brand_slides setting.
	// Lets the admin pick multiple images from the media library and edit a
	// headline + subheading per slide. Serializes to the same JSON array shape
	// consumed by the main app's AuthShell: [{image, headline, subheading}].
	//
	// Uses the full MediaPickerModal (Browse / Upload / Stock) so the admin can
	// pick existing media, upload a new file, or import a stock photo — all
	// without leaving the settings page.
	import { t } from '$lib/stores/i18n.svelte';
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';
	import { Plus, Trash2, GripVertical } from 'lucide-svelte';

	interface Slide {
		image: string;
		headline: string;
		subheading: string;
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

	// --- Media picker (full MediaPickerModal: Browse / Upload / Stock) ---
	let pickerOpen = $state(false);

	function openPicker() {
		pickerOpen = true;
	}

	// MediaPickerModal calls onselect with {id, url, filename} for a single
	// image. Append it as a new slide with empty headline/subheading.
	function handleMediaSelect(media: { id: string; url: string; filename: string }) {
		const next: Slide[] = [
			...slides,
			{ image: media.url, headline: '', subheading: '' }
		];
		emit(next);
	}

	// --- Replace a slide's image (opens picker for an existing slide) ---
	let replaceIndex = $state(-1);

	function openReplacePicker(index: number) {
		replaceIndex = index;
		pickerOpen = true;
	}

	function handleReplace(media: { id: string; url: string; filename: string }) {
		if (replaceIndex >= 0 && replaceIndex < slides.length) {
			const next = slides.map((s, i) =>
				i === replaceIndex ? { ...s, image: media.url } : s
			);
			emit(next);
		}
		replaceIndex = -1;
	}

	// Route the MediaPickerModal onselect to add or replace depending on context.
	function onMediaSelect(media: { id: string; url: string; filename: string }) {
		if (replaceIndex >= 0) {
			handleReplace(media);
		} else {
			handleMediaSelect(media);
		}
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
				<!-- Thumbnail (click to replace via media picker) -->
				<div class="flex shrink-0 flex-col items-center gap-1">
					<button
						type="button"
						class="h-20 w-28 overflow-hidden rounded-md border transition-opacity hover:opacity-80 disabled:opacity-100"
						style="border-color: var(--color-border)"
						disabled={disabled}
						onclick={() => openReplacePicker(i)}
						title={t('settings.auth_slides_replace', 'Click to replace image')}
						aria-label={t('settings.auth_slides_replace', 'Click to replace image')}
					>
						<img src={slide.image} alt={slide.headline || `Slide ${i + 1}`} class="h-full w-full object-cover" />
					</button>
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
						oninput={(e) => updateField(i, 'subheading', (e.target as HTMLTextAreaElement).value)}
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
		{t('settings.auth_slides_add', 'Add image from media library')}
	</button>
</div>

<!-- Full media picker: Browse Library / Upload New / Stock Photos -->
<MediaPickerModal bind:open={pickerOpen} onselect={onMediaSelect} accept="image/*" />
