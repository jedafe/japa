<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Public source proposal page — a standalone deep link for the same
	// ProposeSourceForm that renders inside ProposeSourceModal on /sources.
	import { t } from '$lib/i18n';
	import ProposeSourceForm from '$lib/components/source/ProposeSourceForm.svelte';
	import { Rss } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('sources.submit.title', 'Propose a source'));
</script>

<svelte:head>
	<title>{title}</title>
</svelte:head>

<div class="mx-auto max-w-xl p-4">
	<h1 class="flex items-center gap-2 text-xl font-bold">
		<Rss size={20} class="text-[var(--color-primary)]" />
		{title}
	</h1>
	<p class="mt-1 text-sm text-[var(--color-text-muted)]">
		{t('sources.submit.description', 'Suggest a new source for the directory. Proposals are reviewed by admins before going live.')}
	</p>

	<div class="mt-4">
		<ProposeSourceForm meta={data.meta} />
	</div>
</div>
