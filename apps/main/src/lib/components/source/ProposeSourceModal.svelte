<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Modal shell for the public source proposal form — mirrors
	// SpaceCreationModal (bits-ui Dialog, icon header, close button). The
	// form itself lives in ProposeSourceForm so the /sources/submit page
	// and this modal share one implementation.
	import { Dialog } from 'bits-ui';
	import { Rss, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import ProposeSourceForm from './ProposeSourceForm.svelte';
	import type { SourceMeta } from '$lib/types/sources';

	let {
		open = $bindable(false),
		meta
	}: {
		open?: boolean;
		meta: SourceMeta;
	} = $props();
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-[560px] -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl max-h-[90vh] overflow-y-auto">
			<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]">
				<X size={20} />
			</Dialog.Close>

			<div class="mb-1 flex items-center gap-2">
				<Rss size={20} class="text-[var(--color-primary)]" />
				<h2 class="text-lg font-semibold text-[var(--color-text)]">
					{t('sources.submit.title', 'Propose a source')}
				</h2>
			</div>
			<p class="mb-4 text-sm text-[var(--color-text-subtle)]">
				{t('sources.submit.description', 'Suggest a new source for the directory. Proposals are reviewed by admins before going live.')}
			</p>

			<!-- Remount on each open so a reopened modal starts on a fresh form -->
			{#if open}
				<ProposeSourceForm {meta} />
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
