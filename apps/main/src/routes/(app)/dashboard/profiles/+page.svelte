<script lang="ts">
	// Ref: japaconnect-engineering/sections/33-professional-profiles.md (my profiles)
	// Lists the professional profiles owned by the current user, with a link
	// to the per-profile admin dashboard at /dashboard/profiles/{id}.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Briefcase, Plus, ShieldCheck } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const profiles = $derived(data.profiles);
</script>

<svelte:head>
	<title>{t('profiles.dashboard.title')} — JapaConnect</title>
</svelte:head>

<section class="space-y-4">
	<header class="flex items-center justify-between">
		<div>
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('profiles.dashboard.title')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('profiles.dashboard.subtitle')}</p>
		</div>
		<a
			href="/dashboard/profiles/new"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
		>
			<Plus class="h-4 w-4" />
			{t('profiles.dashboard.new')}
		</a>
	</header>

	{#if profiles.length === 0}
		<EmptyState
			icon={Briefcase}
			title={t('profiles.dashboard.empty_title')}
			body={t('profiles.dashboard.empty_body')}
		/>
	{:else}
		<ul class="space-y-2">
			{#each profiles as p (p.id)}
				<li>
					<a
						href="/dashboard/profiles/{p.id}"
						class="group flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="flex min-w-0 items-center gap-3">
							<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
								<Briefcase class="h-5 w-5" />
							</div>
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">
									{p.name}
									{#if p.verified}
										<ShieldCheck class="ml-1 inline h-3.5 w-3.5 text-[var(--color-primary)]" />
									{/if}
								</p>
								<p class="truncate text-xs text-[var(--color-muted)]">
									/{p.slug} · {t('profiles.dashboard.followers').replace('{n}', String(p.follower_count))}
								</p>
							</div>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>
