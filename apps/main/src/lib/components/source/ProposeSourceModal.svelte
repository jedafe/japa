<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Modal shell for the public source proposal form — mirrors
	// SpaceCreationModal (bits-ui Dialog, icon header, close button). The
	// form itself lives in ProposeSourceForm so the /sources/submit page
	// and this modal share one implementation.
	import { Modal } from '$lib/components/ui';
	import { Rss } from 'lucide-svelte';
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

<Modal bind:open icon={Rss} title={t('sources.submit.title', 'Propose a source')} description={t('sources.submit.description', 'Suggest a new source for the directory. Proposals are reviewed by admins before going live.')} overlayClass="backdrop-blur-sm" size="lg">
	<!-- Remount on each open so a reopened modal starts on a fresh form -->
	{#if open}
		<ProposeSourceForm {meta} />
	{/if}
</Modal>
