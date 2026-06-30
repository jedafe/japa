<script lang="ts">
	import type { HealthStatus } from '$lib/types';
	import { MetadataHead, JsonLd, createWebsiteSchema } from '$lib/seo';
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowRight, Globe, Users, BookOpen, Shield } from 'lucide-svelte';

	let { data }: { data: HealthStatus } = $props();

	const status = $derived(data.healthy ? t('landing.operational') : t('landing.degraded'));
	const label = $derived(data.healthy ? 'ok' : 'warn');

	const metadata = $derived({
		title: t('nav.home'),
		description: t('landing.tagline'),
		content_type: 'website'
	});

	const websiteSchema = $derived(createWebsiteSchema());

	const features = [
		{ icon: Globe, title: t('landing.feature_community'), desc: t('landing.feature_community_desc') },
		{ icon: Users, title: t('landing.feature_spaces'), desc: t('landing.feature_spaces_desc') },
		{ icon: BookOpen, title: t('landing.feature_learn'), desc: t('landing.feature_learn_desc') },
		{ icon: Shield, title: t('landing.feature_trusted'), desc: t('landing.feature_trusted_desc') }
	];
</script>

<MetadataHead {metadata} />
<JsonLd schema={websiteSchema} />

<main class="relative overflow-hidden">
	<!-- Hero Section -->
	<div class="relative px-6 pb-20 pt-16 sm:pt-24">
		<!-- Background decoration -->
		<div class="absolute inset-0 -z-10">
			<div
				class="absolute left-1/2 top-0 -z-10 h-[500px] w-[800px] -translate-x-1/2 opacity-[0.07]"
				style="background: radial-gradient(ellipse at center, var(--color-primary) 0%, transparent 70%);"
			></div>
		</div>

		<div class="mx-auto max-w-3xl text-center">
			<div
				class="animate-fade-in inline-flex items-center gap-2 rounded-full border px-4 py-1.5 text-sm font-medium"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-muted)"
			>
				<span
					class="h-2 w-2 rounded-full"
					style="background-color: {data.healthy ? 'var(--color-success)' : 'var(--color-warning)'}"
				></span>
				{status}
			</div>

			<h1
				class="animate-slide-up mt-6 text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl"
				style="color: var(--color-foreground)"
			>
				JapaConnect
			</h1>
			<p
				class="animate-slide-up mx-auto mt-5 max-w-xl text-lg leading-relaxed sm:text-xl"
				style="color: var(--color-muted); animation-delay: 100ms"
			>
				{t('landing.tagline')}
			</p>

			<div class="animate-slide-up mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row" style="animation-delay: 200ms">
				<a
					href="/register"
					class="btn btn-primary px-8 py-3 text-base"
				>
					{t('landing.get_started')}
					<ArrowRight class="h-4 w-4" />
				</a>
				<a
					href="/feed"
					class="btn btn-secondary px-8 py-3 text-base"
				>
					{t('landing.explore_feed')}
				</a>
			</div>
		</div>
	</div>

	<!-- Features Grid -->
	<section class="mx-auto max-w-5xl px-6 pb-20">
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			{#each features as feature, i}
				{@const Icon = feature.icon}
				<div
					class="card-interactive group rounded-2xl border p-6"
					style="border-color: var(--color-border); background: var(--color-surface); animation-delay: {i * 100}ms"
				>
					<div
						class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl transition-colors"
						style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)"
					>
						<Icon class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<h3 class="text-base font-semibold" style="color: var(--color-foreground)">
						{feature.title}
					</h3>
					<p class="mt-1.5 text-sm leading-relaxed" style="color: var(--color-muted)">
						{feature.desc}
					</p>
				</div>
			{/each}
		</div>
	</section>

	<!-- Health Status Card -->
	<section class="mx-auto max-w-3xl px-6 pb-20">
		<div
			class="rounded-2xl border p-6"
			style="border-color: var(--color-border); background: var(--gradient-surface)"
		>
			<div class="flex items-center gap-4">
				<div
					class="flex h-12 w-12 items-center justify-center rounded-xl"
					style="background: color-mix(in srgb, {data.healthy ? 'var(--color-success)' : 'var(--color-warning)'} 12%, transparent)"
				>
					<span
						class="h-3 w-3 rounded-full"
						style="background-color: {data.healthy ? 'var(--color-success)' : 'var(--color-warning)'}"
					></span>
				</div>
				<div>
					<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
						{t('landing.health')}
					</h2>
					<p class="text-sm" style="color: var(--color-muted)">
						<span class="font-medium" data-testid="health-status">{status}</span>
						<span data-testid="health-badge" class="sr-only">{label}</span>
					</p>
				</div>
			</div>
		</div>
	</section>
</main>