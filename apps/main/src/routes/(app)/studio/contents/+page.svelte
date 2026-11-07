<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §11 — Contents tab:
	// member content manager (Published / Drafts / Scheduled), moved from
	// /dashboard/content.
	import { t } from '$lib/i18n';
	import ContentManagerView from '$lib/components/studio/ContentManagerView.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<SeoHead title={`${t('studio.tabs.contents', 'Contents')} — ${t('studio.title', 'Studio — {site}')}`} robots="`noindex`" description={t('studio.contents_description', 'Review and manage generated content.')} />

<ContentManagerView {data} />
