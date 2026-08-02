<script lang="ts">
	import { t } from '$lib/i18n';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import MetricCard from '$lib/components/hub/MetricCard.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { FileText, Wallet, Bell, Users } from 'lucide-svelte';

	// Analytics fields match the backend MeAnalytics struct (§50):
	// total_content, draft_content, published_content, total_bookmarks,
	// total_followers, total_following.
	interface Analytics {
		total_content?: number;
		draft_content?: number;
		published_content?: number;
		total_bookmarks?: number;
		total_followers?: number;
		total_following?: number;
	}
	interface RecentItem {
		id: string;
		title?: string;
		slug?: string;
		content_type?: string;
		status?: string;
		created_at?: string;
	}
	interface WalletSummary {
		credits?: number;
		points?: number;
	}
	interface UnreadSummary {
		count?: number;
	}
	interface OverviewData {
		analytics: Analytics | null;
		recent: RecentItem[];
		wallet?: WalletSummary | null;
		unread?: UnreadSummary | null;
	}

	let { data }: { data: unknown } = $props();
	const overview = $derived(data as OverviewData | null);
	const analytics = $derived(overview?.analytics ?? {});
	const recent = $derived(overview?.recent ?? []);
	const wallet = $derived(overview?.wallet ?? null);
	const unread = $derived(overview?.unread ?? null);

	// Build the canonical URL for a content item — blog posts use /blog/{slug},
	// other content types use /content/{slug}. Falls back to /content/{id}.
	function contentUrl(item: RecentItem): string {
		const idOrSlug = item.slug ?? item.id;
		if (item.content_type === 'blog_post') return `/blog/${idOrSlug}`;
		return `/content/${idOrSlug}`;
	}

	function formatDate(iso: string | undefined): string {
		if (!iso) return '';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '' : d.toLocaleDateString();
	}
</script>

<section class="space-y-6">
	<!-- Content metrics -->
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<MetricCard labelKey="dashboard.overview.content_count" value={analytics.total_content ?? 0} />
		<MetricCard labelKey="dashboard.overview.draft_count" value={analytics.draft_content ?? 0} />
		<MetricCard labelKey="dashboard.overview.published_count" value={analytics.published_content ?? 0} />
		<MetricCard labelKey="dashboard.overview.bookmark_count" value={analytics.total_bookmarks ?? 0} />
	</div>

	<!-- Phase 18.01 — wallet / unread / follower summary cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<a
			href="/dashboard/wallet"
			class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
		>
			<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Wallet size={18} />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block text-xs text-[var(--color-muted)]">{t('dashboard.overview.wallet', 'Wallet')}</span>
				<span class="block text-sm font-medium text-[var(--color-text)]">
					{#if wallet}
						{wallet.credits ?? 0} {t('dashboard.overview.credits', 'credits')}
					{:else}
						—
					{/if}
				</span>
			</span>
		</a>

		<a
			href="/notifications"
			class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
		>
			<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Bell size={18} />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block text-xs text-[var(--color-muted)]">{t('dashboard.overview.unread', 'Unread')}</span>
				<span class="block text-sm font-medium text-[var(--color-text)]">
					{unread?.count ?? 0}
				</span>
			</span>
		</a>

		<a
			href="/dashboard/relationships"
			class="flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
		>
			<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Users size={18} />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block text-xs text-[var(--color-muted)]">{t('dashboard.overview.followers', 'Followers')}</span>
				<span class="block text-sm font-medium text-[var(--color-text)]">
					{analytics.total_followers ?? 0}
				</span>
			</span>
		</a>
	</div>

	<DashboardPanel titleKey="dashboard.overview.recent">
		{#if recent.length === 0}
			<EmptyState icon={FileText} title={t('dashboard.overview.recent_empty')} />
		{:else}
			<ul class="space-y-2">
				{#each recent as item (item.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2">
						<a href={contentUrl(item)} class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)] hover:underline">
							{item.title ?? t('dashboard.overview.untitled')}
						</a>
						<span class="ml-3 shrink-0 text-xs text-[var(--color-muted)]">{formatDate(item.created_at)}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</DashboardPanel>

	<div class="flex flex-wrap gap-3">
		<a href="/dashboard/content" class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
			{t('dashboard.nav.content')}
		</a>
		<a href="/dashboard/relationships" class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
			{t('dashboard.nav.relationships')}
		</a>
		<a href="/dashboard/learning" class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
			{t('dashboard.nav.learning')}
		</a>
	</div>
</section>
