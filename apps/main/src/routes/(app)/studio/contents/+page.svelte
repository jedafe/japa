<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §11 — Contents tab:
	// member content manager (Published / Drafts / Scheduled), moved from
	// /dashboard/content.
	import { t } from '$lib/i18n';
	import ContentManagerView from '$lib/components/studio/ContentManagerView.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>{t('studio.tabs.contents', 'Contents')} — {t('studio.title', 'Studio — {site}')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<ContentManagerView {data} />
