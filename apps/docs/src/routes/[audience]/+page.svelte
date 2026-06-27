<script lang="ts">
	import { page } from '$app/state';
	import { FileText, ChevronRight } from 'lucide-svelte';

	const { data } = $props<{ data: { pages: Array<{ id: string; title: string; slug: string; review_status: string; position: number }> } }>();

	const sectionLabels: Record<string, string> = {
		users: 'User Guide',
		premium: 'Premium',
		organizations: 'Organizations',
		moderators: 'Moderators',
		api: 'API Reference',
		admin: 'Admin',
	};

	let currentAudience = $derived(page.params.audience ?? '');
	let label = $derived(sectionLabels[currentAudience] ?? currentAudience);
</script>

<svelte:head>
	<title>{label} — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-3xl">
	<h1 class="text-3xl font-bold tracking-tight">{label}</h1>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">Documentation for the {label.toLowerCase()} section.</p>

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
			<p class="py-8 text-center" style="color: var(--color-muted)">No documentation pages available yet.</p>
		{/each}
	</div>
</div>
