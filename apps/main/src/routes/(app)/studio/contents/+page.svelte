<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §11 — Contents tab is a
	// future phase (member drafts/queue management); placeholder until built.
	import { t } from '$lib/i18n';
	import { FolderOpen } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
</script>

<svelte:head>
	<title>{t('studio.tabs.contents', 'Contents')} — {t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<EmptyState
	icon={FolderOpen}
	title={t('studio.contents.title', 'Contents')}
	body={t('studio.contents.coming_soon', 'Your drafts and published content will be managed here soon.')}
/>
