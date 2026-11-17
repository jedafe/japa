<script lang="ts">
	/**
	 * Remote fediverse post card — §126 M5 consume-side timeline item.
	 *
	 * Remote posts are read-only projections: content arrives as plain text
	 * (remote markup is stripped on ingest) and there are no local engagement
	 * APIs to wire (no votes/comments/bookmarks), so the card renders the
	 * author's remote identity, the post body, and links out to the origin.
	 *
	 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
	 */
	import { ExternalLink, EyeOff, MessageSquare, Pin, Repeat2 } from 'lucide-svelte';
	import { t, i18n } from '#lib/i18n/index.js';
	import { relativeTime } from '#lib/feed/time.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { RemoteTimelineItem } from '#lib/types/index.js';

	let { item }: { item: RemoteTimelineItem } = $props();

	const actor = $derived(item.actor);
	const displayName = $derived(actor.display_name || actor.preferred_username || actor.acct || 'Remote user');
	const acct = $derived(actor.acct ?? '');
	const initials = $derived(displayName.slice(0, 2).toUpperCase());
	const time = $derived(
		item.published_at ? relativeTime(item.published_at, new Date(), i18n.locale) : ''
	);
	// Instance domain surfaced as provenance — pulled from the actor URL
	// (always https); falls back to the acct's domain part.
	const instance = $derived.by(() => {
		try {
			return new URL(actor.actor_url).hostname;
		} catch {
			const at = acct.lastIndexOf('@');
			return at >= 0 ? acct.slice(at + 1) : '';
		}
	});

	// CW / sensitive gate — remote `summary` is the content-warning label.
	const hasCw = $derived(item.sensitive || !!item.summary);
	let revealed = $state(false);
</script>

<article
	class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
>
	<header class="flex items-center gap-3">
		{#if actor.avatar_url}
			<img
				src={remoteImageUrl(actor.avatar_url)}
				alt=""
				loading="lazy"
				class="h-10 w-10 shrink-0 rounded-full object-cover"
			/>
		{:else}
			<span
				class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] font-semibold text-[var(--color-primary-contrast)]"
			>{initials}</span>
		{/if}
		<div class="min-w-0 flex-1 leading-tight">
			<p class="truncate text-sm font-semibold text-[var(--color-text)]">{displayName}</p>
			<p class="truncate text-xs text-[var(--color-muted)]">
				{acct}{#if time} · {time}{/if}
			</p>
		</div>
		<div class="flex shrink-0 items-center gap-2 text-xs text-[var(--color-muted)]">
			{#if item.is_pinned}
				<Pin size={13} aria-label={t('feed.global.pinned', 'Pinned')} />
			{/if}
			{#if instance}
				<span class="hidden rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] sm:inline">{instance}</span>
			{/if}
		</div>
	</header>

	{#if item.name}
		<h2 class="text-[15.5px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.name}</h2>
	{/if}

	{#if hasCw && !revealed}
		<button
			type="button"
			onclick={() => (revealed = true)}
			class="flex items-center gap-2 self-start rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:border-[var(--color-primary)]"
		>
			<EyeOff size={13} />
			{item.summary || t('feed.global.sensitive', 'Sensitive content')} — {t('feed.global.show', 'Show')}
		</button>
	{:else}
		<p class="whitespace-pre-wrap break-words text-[14.5px] leading-relaxed text-[var(--color-text)]">{item.content_text}</p>
	{/if}

	{#if item.hashtags?.length}
		<div class="flex flex-wrap gap-1.5">
			{#each item.hashtags.slice(0, 5) as tag, i (i)}
				<span class="text-xs font-medium text-[var(--color-primary-text)]">#{tag}</span>
			{/each}
		</div>
	{/if}

	<footer class="flex items-center gap-4 border-t border-[var(--color-border)] pt-3 text-xs text-[var(--color-muted)]">
		{#if item.in_reply_to_url}
			<span class="flex items-center gap-1"><MessageSquare size={13} /> {t('feed.global.reply', 'Reply')}</span>
		{/if}
		{#if item.share_count > 0}
			<span class="flex items-center gap-1"><Repeat2 size={13} /> {item.share_count}</span>
		{/if}
		{#if item.url}
			<a
				href={item.url}
				target="_blank"
				rel="noopener noreferrer"
				class="ms-auto inline-flex items-center gap-1 hover:text-[var(--color-primary-text)]"
			>
				{t('feed.global.open_remote', 'Open on origin')} <ExternalLink size={12} class="rtl:-scale-x-100" />
			</a>
		{/if}
	</footer>
</article>
