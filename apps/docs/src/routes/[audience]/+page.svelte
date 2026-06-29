<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { FileText, ChevronRight } from 'lucide-svelte';

	const { data } = $props<{ data: { pages: Array<{ id: string; title: string; slug: string; review_status: string; position: number }> } }>();

	const sectionLabels: Record<string, string> = {
		users: 'sections.user_guide',
		premium: 'sections.premium',
		organizations: 'sections.organizations',
		moderators: 'sections.moderators',
		api: 'sections.api_reference',
		admin: 'sections.admin',
	};

	let currentAudience = $derived(page.params.audience ?? '');
	let labelKey = $derived(sectionLabels[currentAudience] ?? currentAudience);
</script>

<svelte:head>
	<title>{$t(labelKey)} — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-3xl">
	<h1 class="text-3xl font-bold tracking-tight">{$t(labelKey)}</h1>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">{$t('audience.description').replace('{audience}', t(labelKey).toLowerCase())}</p>

	<div class="mt-8 space-y-3">
		{#each data.pages as docPage}
			<a
				href="/{currentAudience}/{docPage.slug}"
				class="flex items-center justify-between rounded-lg border p-4 transition-all hover:shadow-sm"
				style="border-color: var(--color-border)"
			>
				<div class="flex items-center gap-3">
					<FileText class="h-4 w-4" style="color: var(--color-muted)" />
					<span class="font-medium">{docPage.title}</span>
				</div>
				<ChevronRight class="h-4 w-4" style="color: var(--color-muted)" />
			</a>
		{:else}
			<p class="py-8 text-center" style="color: var(--color-muted)">{$t('audience.empty')}</p>
		{/each}
	</div>
</div>
