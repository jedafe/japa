<script lang="ts">
	import { Briefcase, Plus, Search } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { page } from '$app/state';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	// svelte-ignore state_referenced_locally
	let searchQuery = $state(data.filters.search || '');

	function handleSearch(e: Event) {
		e.preventDefault();
		const url = new URL(window.location.href);
		if (searchQuery.trim()) {
			url.searchParams.set('search', searchQuery.trim());
		} else {
			url.searchParams.delete('search');
		}
		url.searchParams.delete('offset');
		window.location.href = url.toString();
	}
</script>

<svelte:head>
	<title>Professional Profiles | JapaConnect</title>
	<meta name="description" content="Explore professional profiles of Nigerians preparing for relocation abroad." />
</svelte:head>

{#if !isFeatureEnabled('feature.professional')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('professional.not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('forum.feature_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('forum.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">{t('professional.title')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('professional.subtitle')}
			</p>
		</div>
		<a
			href="/professional/new"
			class="btn btn-primary shrink-0"
		>
			<Plus class="h-4 w-4" />
			{t('professional.create')}
		</a>
	</header>

	<!-- Search -->
	<form onsubmit={handleSearch} class="mb-6 flex gap-2">
		<div class="relative flex-1">
			<Search class="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				bind:value={searchQuery}
				placeholder={t('professional.search_placeholder')}
				class="w-full rounded-xl border py-2.5 pl-10 pr-4 text-sm outline-none transition-all focus:border-[var(--color-primary)] focus:ring-2 focus:ring-offset-1"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground); --tw-ring-color: color-mix(in srgb, var(--color-primary) 25%, transparent)"
			/>
		</div>
		<button
			type="submit"
			class="btn btn-primary"
		>
			{t('professional.search')}
		</button>
	</form>

	{#if data.profiles.length === 0}
		<EmptyState
			icon={Briefcase}
			title={t('professional.none')}
			message={t('professional.none_msg')}
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each data.profiles as profile, i (profile.id)}
				<a
					href="/professional/{profile.slug}"
					class="card-interactive group rounded-2xl border p-5"
					style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-12 w-12 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
							style="background: var(--gradient-primary)"
						>
							{profile.name.charAt(0).toUpperCase()}
						</div>
						<div class="min-w-0 flex-1">
							<h3
								class="truncate text-base font-semibold group-hover:underline"
								style="color: var(--color-foreground)"
							>
								{profile.name}
							</h3>
							<p class="truncate text-sm" style="color: var(--color-muted)">
								{profile.headline}
							</p>
						</div>
					</div>

					{#if profile.skills.length > 0}
						<div class="mt-4 flex flex-wrap gap-1.5">
							{#each profile.skills.slice(0, 5) as skill}
								<span
									class="rounded-full px-2.5 py-0.5 text-xs font-medium"
									style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
								>
									{skill}
								</span>
							{/each}
							{#if profile.skills.length > 5}
								<span class="rounded-full px-2 py-0.5 text-xs font-medium" style="color: var(--color-muted)">+{profile.skills.length - 5}</span>
							{/if}
						</div>
					{/if}

					<div class="mt-4 flex items-center justify-between border-t pt-4" style="border-color: var(--color-border)">
						<span
							class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium"
							style="background: {profile.is_public ? 'color-mix(in srgb, var(--color-success) 12%, transparent)' : 'color-mix(in srgb, var(--color-muted) 10%, transparent)'}; color: {profile.is_public ? 'var(--color-success)' : 'var(--color-muted)'}"
						>
							<span
								class="h-1.5 w-1.5 rounded-full"
								style="background-color: {profile.is_public ? 'var(--color-success)' : 'var(--color-muted)'}"
							></span>
							{profile.is_public ? t('professional.public') : t('professional.private')}
						</span>
					</div>
				</a>
			{/each}
		</div>

		{#if data.totalPages > 1}
			<div class="mt-8 flex items-center justify-center text-sm font-medium" style="color: var(--color-muted)">
				{t('professional.showing').replace('{count}', String(data.profiles.length)).replace('{total}', String(data.total))}
			</div>
		{/if}
	{/if}
	</main>
{/if}
