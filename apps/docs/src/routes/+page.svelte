<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { BookOpen, Code, Users, Shield, GraduationCap, Building } from 'lucide-svelte';

	const paths = [
		{ slug: 'users', labelKey: 'sections.user_guide', descriptionKey: 'sections.user_guide_desc', icon: BookOpen },
		{ slug: 'premium', labelKey: 'sections.premium', descriptionKey: 'sections.premium_desc', icon: GraduationCap },
		{ slug: 'organizations', labelKey: 'sections.organizations', descriptionKey: 'sections.organizations_desc', icon: Building },
		{ slug: 'moderators', labelKey: 'sections.moderators', descriptionKey: 'sections.moderators_desc', icon: Shield },
		{ slug: 'api', labelKey: 'sections.api_reference', descriptionKey: 'sections.api_reference_desc', icon: Code },
		{ slug: 'admin', labelKey: 'sections.admin', descriptionKey: 'sections.admin_desc', icon: Users },
	];
</script>

<svelte:head>
	<title>{$t('landing.title')}</title>
</svelte:head>

<div class="mx-auto max-w-4xl">
	<h1 class="text-4xl font-bold tracking-tight">{$t('landing.title')}</h1>
	<p class="mt-3 text-lg" style="color: var(--color-muted)">
		{$t('landing.description')}
	</p>

	<div class="mt-10 grid gap-4 sm:grid-cols-2">
		{#each paths as path}
			<a
				href="/{path.slug}"
				class="group rounded-lg border p-6 transition-all hover:shadow-md"
				style="border-color: var(--color-border)"
			>
				<div class="flex items-start gap-3">
					<svelte:component this={path.icon} class="mt-0.5 h-5 w-5" style="color: var(--color-primary)" />
					<div>
						<h2 class="font-semibold group-hover:text-blue-600">{$t(path.labelKey)}</h2>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">{$t(path.descriptionKey)}</p>
					</div>
				</div>
			</a>
		{/each}
	</div>
</div>
