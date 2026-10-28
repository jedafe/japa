<script lang="ts">
	// Modal dialog for creating a new blog series (POST /series).
	// Mirrors SpaceCreationModal — uses bits-ui Dialog, auto-generates slug
	// from title, and calls onCreated on success instead of navigating.
	import { Modal } from '$lib/components/ui';
	import { Plus, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { createSeries } from '$lib/api/create';
	import { suggestSlug } from '../slug';
	import type { BlogSeries } from '$lib/types';

	interface Props {
		open?: boolean;
		/** Called with the created series on success. */
		onCreated?: (series: BlogSeries) => void;
	}
	let { open = $bindable(false), onCreated }: Props = $props();

	let title = $state('');
	let slug = $state('');
	let description = $state('');
	let slugTouched = $state(false);
	let submitting = $state(false);
	let error = $state('');

	function reset() {
		title = '';
		slug = '';
		description = '';
		slugTouched = false;
		error = '';
	}

	function onTitleInput(e: Event) {
		title = (e.currentTarget as HTMLInputElement).value;
		if (!slugTouched) slug = suggestSlug(title);
	}

	function onSlugInput(e: Event) {
		slug = (e.currentTarget as HTMLInputElement).value;
		slugTouched = true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const tTitle = title.trim();
		const tSlug = slug.trim();
		if (!tTitle || !tSlug) {
			error = t('create.series.error_fields');
			return;
		}
		submitting = true;
		const res = await createSeries({
			title: tTitle,
			slug: tSlug,
			description: description.trim() || undefined
		});
		submitting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			open = false;
			onCreated?.(res.data);
		}
	}
</script>

<Modal bind:open onOpenChange={(isOpen) => { if (!isOpen) reset(); }} icon={Plus} title={t('create.series.create_title')} overlayClass="backdrop-blur-sm" size="max-w-[480px]">
			<form onsubmit={submit} class="space-y-3">
				<div>
					<label for="series-title" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('create.series.title_label')}</label>
					<input
						id="series-title"
						value={title}
						oninput={onTitleInput}
						placeholder={t('create.series.title_placeholder')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						disabled={submitting}
						aria-describedby="series-title-hint"
					/>
					<p id="series-title-hint" class="mt-1 text-xs text-[var(--color-muted)]">{t('create.series.title_hint', 'The public name of the series.')}</p>
				</div>

				<div>
					<label for="series-slug" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('create.series.slug_label')}</label>
					<input
						id="series-slug"
						value={slug}
						oninput={onSlugInput}
						placeholder={t('create.series.slug_placeholder')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						disabled={submitting}
						aria-describedby="series-slug-hint"
					/>
					<p id="series-slug-hint" class="mt-1 text-xs text-[var(--color-muted)]">{t('create.series.slug_hint', 'URL-safe identifier — auto-generated from the title.')}</p>
				</div>

				<div>
					<label for="series-desc" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('create.series.description_label')}</label>
					<input
						id="series-desc"
						value={description}
						oninput={(e) => (description = e.currentTarget.value)}
						placeholder={t('create.series.description_placeholder')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						disabled={submitting}
						aria-describedby="series-desc-hint"
					/>
					<p id="series-desc-hint" class="mt-1 text-xs text-[var(--color-muted)]">{t('create.series.desc_hint', 'Optional — what this series is about.')}</p>
				</div>

				{#if error}
					<p class="text-sm text-red-500" role="alert">{error}</p>
				{/if}

				<div class="flex justify-end gap-2 pt-1">
					<button type="button" onclick={() => (open = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
						{t('common.cancel')}
					</button>
					<button type="submit" disabled={submitting} class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50">
						{#if submitting}
							<Loader2 size={14} class="animate-spin" />
							{t('create.series.creating')}
						{:else}
							<Plus size={14} />
							{t('create.series.create_button')}
						{/if}
					</button>
				</div>
			</form>
</Modal>
