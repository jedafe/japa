<script lang="ts" generics="T extends Record<string, unknown>">
	import type { Snippet } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { confirmDialog } from '@japaconnect/ui';
	import { t } from '#lib/i18n/index.js';
	import type { DirtyForm } from './useDirtyForm.svelte';

	interface Props {
		titleKey: string;
		descKey?: string;
		form?: DirtyForm<T>;
		/** Hide the section's own h1/desc header — used when the section is
		 *  embedded as a tab inside a parent section (e.g. /settings/ai). */
		hideHeader?: boolean;
		children?: Snippet;
	}
	let { titleKey, descKey, form, hideHeader = false, children }: Props = $props();

	let saved = $state(false);
	let saveTimer: ReturnType<typeof setTimeout> | undefined;

	// Async confirm can't block a navigation — cancel first, then re-issue
	// the nav if the user confirms. bypassGuard lets the re-issued nav pass.
	let bypassGuard = false;
	beforeNavigate((nav) => {
		if (nav.shallow && nav.type === 'goto') return;
		if (!form?.dirty || bypassGuard || nav.type === 'leave') return;
		nav.cancel();
		const target = nav.to?.url;
		confirmDialog(t('settings.unsaved_warning'), {
			confirmLabel: t('common.leave', 'Leave')
		}).then(async (ok) => {
			if (!ok) return;
			bypassGuard = true;
			try {
				if (target) await goto(target);
			} finally {
				bypassGuard = false;
			}
		});
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
	{#if !hideHeader}
		<header class="mb-4">
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t(titleKey)}</h1>
			{#if descKey}<p class="text-sm text-[var(--color-muted)]">{t(descKey)}</p>{/if}
		</header>
	{/if}
	{@render children?.()}
	{#if form && (form.dirty || saved)}
		<div
			class="sticky bottom-0 mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] py-3"
		>
			{#if form.error}
				<span class="me-auto text-xs text-[var(--color-danger,#dc2626)]">{form.error.message}</span>
			{/if}
			{#if saved}
				<span class="me-auto text-xs" style="color: var(--color-success, #16a34a);">{t('settings.action.saved', 'Saved ✓')}</span>
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
