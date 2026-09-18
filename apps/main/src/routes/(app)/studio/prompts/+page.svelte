<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab.
	// Stub for Phase 1; the prompt builder + import flow lands in Phase 3.
	import { t } from '$lib/i18n';
	import { Wand2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
</script>

<svelte:head>
	<title>{t('studio.tabs.prompts', 'Prompts')} — {t('app.name', 'JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<EmptyState
	icon={Wand2}
	title={t('studio.tabs.prompts', 'Prompts')}
	body={t('studio.tabs.coming_soon', 'Coming soon')}
/>
