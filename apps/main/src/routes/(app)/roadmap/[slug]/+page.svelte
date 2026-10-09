<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// Roadmap item detail — status stepper, vote pill, category chip, target
	// window, feature-flag rollout badge, description, and the §49 comment
	// thread (via the item's shadow contents row). Moderators carrying
	// roadmap.moderate get the same inline status select as board cards.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { tiptapBodyToHtml } from '$lib/tiptap/serialize';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import AdSlot from '$lib/components/ui/AdSlot.svelte';
	import VoteButton from '$lib/components/roadmap/VoteButton.svelte';
	import StatusBadge from '$lib/components/roadmap/StatusBadge.svelte';
	import StatusSelect from '$lib/components/roadmap/StatusSelect.svelte';
	import RoadmapCard from '$lib/components/roadmap/RoadmapCard.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { ArrowLeft, Flag, Pin } from 'lucide-svelte';
	import { roadmapPublicStatuses, roadmapStatusLabel } from '$lib/enumOptions';

	let { data } = $props();
	const item = $derived(data.item);

	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);

	// body is base64 TipTap JSON for submissions created via /roadmap/submit
	// (encodeReplyBody) — tiptapBodyToHtml returns '' for anything that is not
	// a decodable doc, in which case the raw body renders as pre-wrapped text.
	const bodyHtml = $derived(
		tiptapBodyToHtml(item.body, {
			deletedUserIds,
			deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
		})
	);

	const currentStep = $derived(roadmapPublicStatuses.indexOf(item.status));

	// Enriched rollout badge for flag-linked items (unified-rollout spec §5.3):
	// disabled → "Paused"; partial rollout → "Rolling out — {p}%"; fully rolled
	// out or shipped phase → "Live"; otherwise the phase label. A missing
	// flagDetail (endpoint failure or unknown key) degrades to the generic
	// "Rolling out" badge rather than dropping it.
	const flagBadgeLabel = $derived.by((): string | null => {
		if (!item.feature_flag_key) return null;
		const flag = data.flagDetail;
		if (!flag) return t('roadmap.rolling_out', 'Rolling out');
		if (!flag.enabled) return t('roadmap.rollout_paused', 'Paused');
		const pct = flag.rolloutPercentage;
		if (pct > 0 && pct < 100) {
			return t('roadmap.rollout_partial', 'Rolling out — {percent}%', { percent: pct });
		}
		if (pct === 100 || flag.phase === 'shipped') {
			return t('roadmap.rollout_live', 'Live');
		}
		return t(`roadmap.phase_${flag.phase}`, flag.phase.replace(/_/g, ' '));
	});
	const createdDate = $derived(
		new Date(item.created_at).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		})
	);
	const shippedDate = $derived(
		item.shipped_at
			? new Date(item.shipped_at).toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'long',
					day: 'numeric'
				})
			: null
	);
</script>

<svelte:head>
	<title>{item.title} — {t('roadmap.title', 'Product Roadmap')} — JapaConnect</title>
	<meta
		name="description"
		content={item.summary || t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')}
	/>
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={item.title} />
	<meta
		property="og:description"
		content={item.summary || t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')}
	/>
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={item.title} />
	<meta
		name="twitter:description"
		content={item.summary || t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')}
	/>
</svelte:head>

<section class="mx-auto max-w-3xl space-y-6 px-4 py-8">
	<a
		href="/roadmap"
		class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('roadmap.back_to_board', 'Back to roadmap')}
	</a>

	<article
		class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6"
	>
		<div class="flex gap-4">
			<VoteButton {item} authed={data.authed} />

			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					<StatusBadge status={item.status} />
					{#if item.is_flagship}
						<span
							class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[11px] font-medium text-[var(--color-primary)]"
						>
							<Pin size={11} />
							{t('roadmap.flagship', 'Now building')}
						</span>
					{/if}
					{#if flagBadgeLabel}
						<span
							class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-muted)]"
							title={item.feature_flag_key}
						>
							<Flag size={11} />
							{flagBadgeLabel}
						</span>
					{/if}
				</div>

				<h1 class="mt-2 text-2xl font-semibold leading-tight text-[var(--color-text)]">
					{item.title}
				</h1>

				<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
					{#if item.category_name}
						<span class="inline-flex items-center gap-1">
							{#if item.category_color}
								<span
									class="inline-block h-2 w-2 rounded-full"
									style:background-color={item.category_color}
								></span>
							{/if}
							{item.category_name}
						</span>
					{/if}
					{#if item.target_window}
						<span>{t('roadmap.target_window', 'Target')}: {item.target_window}</span>
					{/if}
					<span>{t('roadmap.added_on', 'Added {date}', { date: createdDate })}</span>
					{#if shippedDate}
						<span>{t('roadmap.shipped_on', 'Shipped {date}', { date: shippedDate })}</span>
					{/if}
				</div>
			</div>

			{#if data.canModerate}
				<StatusSelect itemId={item.id} status={item.status} featureFlagKey={item.feature_flag_key} />
			{/if}

			{#if item.is_submitter && item.status === 'declined'}
				<AppealButton entityType="roadmap_item" entityId={item.id} />
			{/if}
		</div>

		<!-- Status stepper: the public lifecycle stages (§113). -->
		{#if currentStep >= 0}
			<ol class="mt-6 flex items-center gap-1" aria-label={t('roadmap.progress', 'Progress')}>
				{#each roadmapPublicStatuses as step, i (step)}
					<li class="flex flex-1 flex-col items-center gap-1">
						<span
							class="h-1.5 w-full rounded-full {i <= currentStep
								? 'bg-[var(--color-primary)]'
								: 'bg-[var(--color-border)]'}"
						></span>
						<span
							class="text-[11px] {i === currentStep
								? 'font-semibold text-[var(--color-primary)]'
								: i < currentStep
									? 'text-[var(--color-text-subtle)]'
									: 'text-[var(--color-muted)]'}"
						>
							{roadmapStatusLabel(t, step)}
						</span>
					</li>
				{/each}
			</ol>
		{/if}

		{#if bodyHtml}
			<div
				class="prose mt-6 max-w-none text-[var(--color-text)]"
				use:hydrateLinkPreviews
				use:hydrateMentions
				use:hydrateVideoEmbeds
			>
				{@html bodyHtml}
			</div>
		{:else if item.body}
			<div class="mt-6 whitespace-pre-wrap text-sm text-[var(--color-text)]">{item.body}</div>
		{:else if item.summary}
			<p class="mt-6 text-sm text-[var(--color-text-subtle)]">{item.summary}</p>
		{/if}
	</article>

	{#if item.content_id}
		<CommentThread
			comments={data.comments}
			contentId={item.content_id}
			contentType="roadmap"
			authed={data.authed}
			currentUserId={data.currentUserId}
			canModerate={data.canModerate}
			loadError={data.commentsLoadError}
			{deletedUserIds}
		/>
		<!-- §25: engagement-area ad slot. -->
		<AdSlot slot="comments_below" format="banner" />
	{/if}

	{#if data.related.length > 0}
		<section class="space-y-3">
			<h2 class="text-lg font-semibold text-[var(--color-text)]">
				{t('roadmap.related', 'Related items')}
			</h2>
			{#each data.related as rel (rel.id)}
				<RoadmapCard item={rel} authed={data.authed} canModerate={data.canModerate} />
			{/each}
		</section>
	{/if}
</section>
