<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { BookOpen, Code, Users, Shield, GraduationCap, Building, ArrowRight } from 'lucide-svelte';

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
	<title>{t('landing.title')}</title>
</svelte:head>

<div class="mx-auto max-w-4xl animate-fade-in px-4 py-12 sm:px-6">
	<div class="text-center">
		<h1 class="text-4xl font-bold tracking-tight sm:text-5xl">{t('landing.title')}</h1>
		<p class="mx-auto mt-4 max-w-2xl text-lg leading-relaxed" style="color: var(--color-muted)">
			{t('landing.description')}
		</p>
	</div>

	<div class="mt-12 grid gap-4 sm:grid-cols-2">
		{#each paths as path, i}
			{@const Icon = path.icon}
			<a
				href="/{path.slug}"
				class="card-interactive group relative overflow-hidden rounded-2xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
			>
				<div class="flex items-start gap-4">
					<div
						class="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl transition-colors"
						style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)"
					>
						<svelte:component this={Icon} class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<h2 class="font-semibold transition-colors" style="color: var(--color-foreground)">
								{t(path.labelKey)}
							</h2>
							<ArrowRight class="h-4 w-4 opacity-0 transition-all group-hover:translate-x-0.5 group-hover:opacity-100" style="color: var(--color-primary)" />
						</div>
						<p class="mt-1 text-sm leading-relaxed" style="color: var(--color-muted)">{t(path.descriptionKey)}</p>
					</div>
				</div>
			</a>
		{/each}
	</div>
</div>
