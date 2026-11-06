<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { t, siteName } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import {
		ArrowRight,
		Award,
		BadgePercent,
		BookOpen,
		Building2,
		GraduationCap,
		Landmark,
		Package,
		Plane,
		School,
		Store
	} from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let kinds = $derived(data.kinds ?? []);

	const KIND_ICONS: Record<string, typeof BookOpen> = {
		course: GraduationCap,
		program: School,
		bank: Landmark,
		embassy: Building2,
		visa_route: Plane,
		scholarship: Award,
		organization: Building2,
		product: Package,
		provider: Store,
		deal: BadgePercent
	};

	function kindIcon(kind: string) {
		return KIND_ICONS[kind] ?? BookOpen;
	}
</script>

<SeoHead title={`${t('wiki.entities.title', 'Entity Wiki')} — ${siteName()}`} description={t('wiki.entities.description', 'Community-maintained wiki entries for destinations, services, schools, and organisations — written and kept up to date by members.')} />

<div class="mx-auto w-full max-w-4xl px-4 py-4 sm:px-6">
	<header class="mt-4">
		<p class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			{t('wiki.entities.eyebrow', 'Directory')}
		</p>
		<h1 class="mt-1 text-2xl font-bold text-[var(--color-text)]">{t('wiki.entities.title', 'Entity Wiki')}</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('wiki.entities.subtitle', 'Browse entities by kind — programs, course offerings, banks, embassies, and more.')}
		</p>
	</header>

	{#if kinds.length === 0}
		<div class="mt-8">
			<EmptyState icon={BookOpen} title={t('wiki.entities.empty', 'No entity kinds available')} />
		</div>
	{:else}
		<div class="mt-8 grid grid-cols-1 gap-3 sm:grid-cols-2">
			{#each kinds as k (k.kind)}
				{@const Icon = kindIcon(k.kind)}
				<div class="flex flex-col gap-2">
					<a
						href="/wiki/e/{k.kind}"
						class="group flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
					>
						<span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)] transition-colors group-hover:text-[var(--color-primary)]">
							<Icon size={18} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-semibold text-[var(--color-text)]">{k.label}</span>
							<span class="mt-0.5 block text-xs text-[var(--color-muted)] tabular-nums">
								{t('wiki.entities.count', '{count} entries').replace('{count}', String(k.entity_count))}
							</span>
						</span>
						<ArrowRight size={16} class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary)]" />
					</a>
					{#if k.types?.length}
						<div class="flex flex-wrap gap-1.5 ps-1">
							{#each k.types as ty (ty.slug)}
								<a
									href="/wiki/e/{ty.slug}"
									class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-[11px] font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary)]"
								>
									{ty.label}
									<span class="tabular-nums opacity-70">{ty.entity_count}</span>
								</a>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
