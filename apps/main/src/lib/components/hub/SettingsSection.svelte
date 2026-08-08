<script lang="ts" generics="T extends Record<string, unknown>">
	import type { Snippet } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import type { DirtyForm } from './useDirtyForm.svelte';

	interface Props {
		titleKey: string;
		descKey?: string;
		form?: DirtyForm<T>;
		children?: Snippet;
	}
	let { titleKey, descKey, form, children }: Props = $props();

	let saved = $state(false);
	let saveTimer: ReturnType<typeof setTimeout> | undefined;

	beforeNavigate((nav) => {
		if (form?.dirty && !confirm(t('settings.unsaved_warning'))) nav.cancel();
	});

	async function handleSave() {
		if (!form) return;
		const ok = await form.save();
		if (ok) {
			saved = true;
			clearTimeout(saveTimer);
			saveTimer = setTimeout(() => { saved = false; }, 2500);
		}
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t(titleKey)}</h1>
		{#if descKey}<p class="text-sm text-[var(--color-muted)]">{t(descKey)}</p>{/if}
	</header>
	{@render children?.()}
	{#if form && (form.dirty || saved)}
		<div
			class="sticky bottom-0 mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] py-3"
		>
			{#if form.error}
				<span class="mr-auto text-xs text-[var(--color-danger,#dc2626)]">{form.error.message}</span>
			{/if}
			{#if saved}
				<span class="mr-auto text-xs" style="color: var(--color-success, #16a34a);">{t('settings.action.saved', 'Saved ✓')}</span>
			{/if}
			{#if form.dirty}
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					onclick={() => form.reset()}>{t('settings.action.discard')}</button
				>
				<button
					type="button"
					disabled={form.saving}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					onclick={handleSave}>{t('settings.action.save')}</button
				>
			{/if}
		</div>
	{/if}
</section>
