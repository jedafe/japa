<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { page as pageState } from '$app/state';
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import DocPageForm from '#lib/components/docs/doc-page-form.svelte';
	import { goto } from '$app/navigation';

	const canEdit = $derived(hasPermission('docs.admin'));
	const docId = $derived(pageState.params.id ?? '');
	// Return target passed by the list page (?from=) — internal /docs paths only
	// ('/docs', '/docs/…', '/docs?…' — not '/docsanything').
	const backTo = $derived.by(() => {
		const from = pageState.url.searchParams.get('from') ?? '';
		return from === '/docs' || from.startsWith('/docs/') || from.startsWith('/docs?') ? from : '/docs';
	});
</script>

<svelte:head>
	<title>{t('docs.edit_doc_page')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('docs.edit_doc_page')}
	description={t('docs.admin_documentation_pages_for_developer')}
	actions={[{ label: t('common.back', 'Back'), variant: 'outline', onclick: () => goto(backTo) }]}
/>

{#if canEdit}
	{#key docId}
		<DocPageForm
			{docId}
			onsaved={() => goto(backTo)}
			oncancel={() => goto(backTo)}
		/>
	{/key}
{:else}
	<p class="text-sm" style="color: var(--color-muted)">{t('docs.no_permission', 'You do not have permission to manage documentation.')}</p>
{/if}
