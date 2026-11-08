<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Achievement centre — gamification v2 catalogue (migration 001016).
	// Shield-grid of every active badge grouped by category, with earned /
	// locked / secret states and progress toward each goal (mockup-aligned).
	import { page } from '$app/state';
	import { t, i18n } from '#lib/i18n/index.js';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import {
		Award, Lock, Trophy, Compass, GraduationCap, Briefcase, Users,
		BookOpen, MessageSquare, HeartHandshake, Package, PiggyBank, Plane,
		ShieldCheck, Flame, Star, GitCompare, Bot, Home, FileCheck, Sparkles,
		MapPin, TrendingUp, Flag
	} from 'lucide-svelte';
	import type { IconComponent } from '#lib/components/ui/icons.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { Badge, CollectionBadge } from '#lib/types/index.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Guest view — group the public catalogue by category so it renders
	// identically to the signed-in collection (locked tiles, no earned).
	const guestCategories = $derived.by(() => {
		const order: string[] = [];
		const groups = new Map<string, Badge[]>();
		for (const b of data.allBadges) {
			const key = b.category || 'general';
			if (!groups.has(key)) { groups.set(key, []); order.push(key); }
			groups.get(key)!.push(b);
		}
		return order.map((category) => ({ category, badges: groups.get(category)! }));
	});

	const title = $derived(t('badges.page_title', 'Your Badges — {site}'));
	const description = $derived(
		t('badges.page_description', 'Earn badges as you explore, learn, connect, and achieve your Japa goals.')
	);

	const categoryIcons: Record<string, IconComponent> = {
		getting_started: Flag,
		readiness: Sparkles,
		journey: MapPin,
		countries: Compass,
		education: GraduationCap,
		career: Briefcase,
		community: Users,
		content: BookOpen,
		forum: MessageSquare,
		mentorship: HeartHandshake,
		services: Package,
		financial: PiggyBank,
		travel: Plane,
		trust: ShieldCheck,
		consistency: Flame,
		special: Star,
		comparison: GitCompare,
		academy: GraduationCap,
		housing: Home,
		documents: FileCheck,
		ai: Bot,
		growth: TrendingUp,
		general: Award
	};

	// Rarity → accent color used for the badge shield ring + name.
	const rarityColors: Record<string, string> = {
		common: 'var(--chart-2)',
		uncommon: 'var(--chart-4)',
		rare: 'var(--chart-1)',
		epic: 'var(--chart-5)',
		legendary: '#d97706',
		mythic: '#7c3aed'
	};

	function categoryIcon(slug: string): IconComponent {
		return categoryIcons[slug] ?? Award;
	}

	function categoryLabel(slug: string): string {
		return t('badges.category.' + slug, slug.replace(/_/g, ' '));
	}

	function rarityColor(badge: { rarity?: string; accent_color?: string | null }): string {
		return badge.accent_color || rarityColors[badge.rarity ?? ''] || 'var(--color-primary)';
	}

	function progressPct(badge: CollectionBadge): number {
		if (!badge.goal || badge.goal <= 0) return 0;
		return Math.min(100, Math.round((badge.progress / badge.goal) * 100));
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-3xl px-4 py-6" style="--page-gutter: 1rem">
	<!-- Hero — mirrors the "Your Journey. Your Badges." mockup header. -->
	<header class="mb-6">
		<p class="text-xs font-bold tracking-widest text-[var(--color-primary-text)] uppercase">japa.to</p>
		<h1 class="mt-1 text-2xl font-extrabold text-[var(--color-text)] sm:text-3xl">
			{t('badges.heading_a', 'Your Journey.')}<br />{t('badges.heading_b', 'Your Badges.')}
		</h1>
		<p class="mt-2 max-w-md text-sm text-[var(--color-text-subtle)]">
			{t('badges.subtitle', 'Earn badges as you explore, learn, connect, and achieve your Japa goals.')}
		</p>
	</header>

	{#if data.disabled}
		<EmptyState
			icon={Trophy}
			title={t('badges.disabled', 'Badges are unavailable')}
			body={t('badges.disabled_body', 'The badge system is currently disabled.')}
		/>
	{:else if !data.authed}
		<EmptyState
			icon={Trophy}
			title={t('badges.login_title', 'Start collecting badges')}
			body={t('badges.login_body', 'Log in to track your badge collection and progress.')}
			action={{ label: t('common.login', 'Log in'), href: '/login?redirect=/badges' }}
		/>
		<!-- Full catalogue preview for guests — same category-grouped layout
		     as the signed-in collection, every badge locked, secrets masked. -->
		{#each guestCategories as cat (cat.category)}
			{@const CatIcon = categoryIcon(cat.category)}
			<section class="mb-8 mt-6">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="flex items-center gap-2 text-sm font-bold capitalize text-[var(--color-text)]">
						<CatIcon size={15} class="text-[var(--color-primary-text)]" />
						{categoryLabel(cat.category)}
					</h2>
					<span class="text-xs font-medium text-[var(--color-muted)]">0/{cat.badges.length}</span>
				</div>
				<div class="mb-3 h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
					<div class="h-full rounded-full bg-[var(--color-primary)]" style="width: 0%"></div>
				</div>
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					{#each cat.badges as badge (badge.id)}
						<div
							class="relative flex flex-col items-center rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4 text-center opacity-70"
							title={badge.is_secret ? '' : badge.description}
						>
							{#if badge.is_limited}
								<span class="absolute top-2 right-2 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-[9px] font-bold uppercase text-[var(--color-primary-text)]">
									{t('badges.limited', 'Ltd')}
								</span>
							{/if}
							<div class="grid h-14 w-14 place-items-center overflow-hidden rounded-full border-2 border-[var(--color-border)]">
								{#if badge.icon_url}
									<img src={remoteImageUrl(badge.icon_url)} alt="" class="h-3/4 w-3/4 object-contain opacity-50 grayscale" />
								{:else}
									<Lock size={18} class="text-[var(--color-muted)]" />
								{/if}
							</div>
							<h3 class="mt-2 text-xs font-bold tracking-wide uppercase text-[var(--color-muted)]">
								{badge.is_secret ? '???' : badge.name}
							</h3>
							<p class="mt-1 line-clamp-2 text-[11px] leading-snug text-[var(--color-text-subtle)]">
								{badge.is_secret ? t('badges.secret', 'Secret badge') : badge.description}
							</p>
						</div>
					{/each}
				</div>
			</section>
		{/each}
	{:else if !data.collection}
		<EmptyState
			icon={Trophy}
			title={t('badges.load_error', 'Could not load badges')}
			body={t('badges.load_error_body', 'Please try again later.')}
		/>
	{:else}
		{@const stats = data.collection.stats}
		<!-- Collection stats -->
		<div class="mb-8 grid grid-cols-3 gap-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
				<div class="text-2xl font-extrabold text-[var(--color-primary-text)]">{stats.earned}</div>
				<div class="text-xs font-medium text-[var(--color-text-subtle)]">{t('badges.stat_earned', 'Earned')}</div>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
				<div class="text-2xl font-extrabold text-[var(--color-text)]">{stats.remaining}</div>
				<div class="text-xs font-medium text-[var(--color-text-subtle)]">{t('badges.stat_remaining', 'Remaining')}</div>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
				<div class="text-2xl font-extrabold text-[var(--color-text)]">{stats.percent}%</div>
				<div class="text-xs font-medium text-[var(--color-text-subtle)]">{t('badges.stat_collection', 'Collection')}</div>
			</div>
		</div>

		<!-- Category collections -->
		{#each data.collection.categories as cat (cat.category)}
			{@const CatIcon = categoryIcon(cat.category)}
			<section class="mb-8">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="flex items-center gap-2 text-sm font-bold text-[var(--color-text)] capitalize">
						<CatIcon size={15} class="text-[var(--color-primary-text)]" />
						{categoryLabel(cat.category)}
					</h2>
					<span class="text-xs font-medium text-[var(--color-muted)]">{cat.earned}/{cat.total}</span>
				</div>
				<div class="mb-3 h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
					<div
						class="h-full rounded-full bg-[var(--color-primary)] transition-all"
						style="width: {cat.total > 0 ? Math.round((cat.earned / cat.total) * 100) : 0}%"
					></div>
				</div>

				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					{#each cat.badges as badge (badge.id)}
						{@const accent = rarityColor(badge)}
						<div
							class="relative flex flex-col items-center rounded-[var(--radius-card)] border p-4 text-center transition-colors"
							class:border-[var(--color-border)]={badge.earned}
							class:bg-[var(--color-surface)]={badge.earned}
							class:border-dashed={!badge.earned}
							class:bg-[var(--color-surface-subtle)]={!badge.earned}
							class:opacity-70={!badge.earned}
							title={badge.earned ? badge.description : badge.is_secret ? '' : (badge.description ?? '')}
						>
							{#if badge.is_limited}
								<span class="absolute top-2 right-2 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-[9px] font-bold text-[var(--color-primary-text)] uppercase">
									{t('badges.limited', 'Ltd')}
								</span>
							{/if}

							<!-- Shield mark -->
							<div
								class="grid h-14 w-14 place-items-center overflow-hidden rounded-full border-2"
								style="border-color: {badge.earned ? accent : 'var(--color-border)'}; background: {badge.earned ? `color-mix(in srgb, ${accent} 14%, transparent)` : 'transparent'}"
							>
								{#if badge.is_secret && !badge.earned}
									<Lock size={20} class="text-[var(--color-muted)]" />
								{:else if badge.icon_url}
									<img
										src={remoteImageUrl(badge.icon_url)}
										alt=""
										class="h-3/4 w-3/4 object-contain"
										class:opacity-50={!badge.earned}
										class:grayscale={!badge.earned}
									/>
								{:else if badge.earned}
									<Award size={24} style="color: {accent}" />
								{:else}
									<Lock size={18} class="text-[var(--color-muted)]" />
								{/if}
							</div>

							<h3
								class="mt-2 text-xs font-bold tracking-wide uppercase"
								style="color: {badge.earned ? accent : 'var(--color-muted)'}"
							>
								{badge.name}
							</h3>

							{#if badge.description}
								<p class="mt-1 line-clamp-2 text-[11px] leading-snug text-[var(--color-text-subtle)]">
									{badge.description}
								</p>
							{/if}

							<div class="mt-auto w-full pt-2">
								{#if badge.earned && badge.awarded_at}
									<span class="text-[10px] font-medium text-[var(--color-muted)]">
										{t('badges.earned_on', 'Earned')} {new Date(badge.awarded_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short' })}
									</span>
								{:else if badge.is_secret}
									<span class="text-[10px] font-medium text-[var(--color-muted)]">{t('badges.secret', 'Secret badge')}</span>
								{:else if badge.goal && badge.goal > 0}
									<div class="h-1 w-full overflow-hidden rounded-full bg-[var(--color-surface)]">
										<div class="h-full rounded-full" style="width: {progressPct(badge)}%; background: {accent}"></div>
									</div>
									<span class="mt-1 block text-[10px] text-[var(--color-muted)]">{badge.progress}/{badge.goal}</span>
								{:else}
									<span class="text-[10px] font-medium text-[var(--color-muted)]">{t('badges.locked', 'Locked')}</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</section>
		{/each}

		<!-- Footer note (mockup strip) -->
		<p class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 text-xs text-[var(--color-text-subtle)]">
			<Sparkles size={12} class="mr-1 inline text-[var(--color-primary-text)]" />
			{t('badges.more_coming', 'More badges coming soon! Keep engaging and level up your journey with japa.to')}
		</p>
	{/if}
</div>
