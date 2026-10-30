<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Users, FileText, ScrollText, LogIn, Shield } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import AchievementWidget from '$lib/components/spaces/AchievementWidget.svelte';
	import TipButton from '$lib/components/tips/TipButton.svelte';
	import SpaceFollowListModal from '$lib/components/spaces/SpaceFollowListModal.svelte';
	import type { Space, SpaceMember, SpaceRule, Achievement } from '$lib/types';

	interface Props {
		space: Space;
		/** Space rules — the detail bundle omits this field entirely when the
		 * space has none, so it must default rather than assume an array. */
		rules?: SpaceRule[];
		authed: boolean;
		/** Imgproxy-signed logo URL (from GET /spaces/{slug} logo_url field). */
		logoUrl?: string;
		/** Whether the caller is an approved member — from the detail bundle's
		 * viewer-scoped is_member flag (audit fix: /me/spaces is capped and
		 * lists all spaces for super-admins, so it can't be trusted). */
		initialJoined?: boolean;
		/** Whether the caller is the space owner — sole owners cannot leave
		 * (SPACES-017), so the button renders as a disabled "Owned" state. */
		isOwner?: boolean;
		/** Whether the caller already has a pending join request (is_pending). */
		initialPending?: boolean;
		/** Initial follow state from the server load. */
		following?: boolean;
		/** Called whenever the sidebar mutates follow state, so the parent
		 * page's header Follow button stays in sync. */
		onFollowChange?: (value: boolean) => void;
		/** Pre-fetched achievements so the widget SSRs the same DOM as client. */
		achievements?: Achievement[];
		/** First page of members (joined_at DESC) — rendered by the New
		 * members widget; hidden when empty. */
		members?: SpaceMember[];
	}
	let { space, rules = [], authed, logoUrl = '', initialJoined = false, initialPending = false, isOwner = false, following: followingProp = false, onFollowChange, achievements = [], members = [] }: Props = $props();

	let following = $state(untrack(() => followingProp ?? false));

	// Wiki content: the API stores it as base64-encoded JSON with
	// {title, extract, description, thumbnail, content_url}. Decode and
	// extract the text for the sidebar about widget.
	const wikiExtract = $derived.by(() => {
		if (!space.wiki_content) return null;
		try {
			const decoded = typeof space.wiki_content === 'string'
				? decodeBase64Text(space.wiki_content)
				: String(space.wiki_content);
			try {
				const json = JSON.parse(decoded);
				return json?.extract ?? null;
			} catch {
				// Not JSON — treat as raw HTML, strip tags for sidebar preview.
				return decoded.replace(/<[^>]*>/g, '').trim() || null;
			}
		} catch {
			return null;
		}
	});

	// Join state is hydrated from the server (initialJoined) so the button
	// reflects the caller's actual membership on SSR load instead of always
	// defaulting to "Join" — which previously caused the toggle to LEAVE when
	// the user was already a member (e.g. auto-joined via follow, BL-010).
	let joined = $state(untrack(() => initialJoined));
	// pendingJoin is set when the space is request-only and the caller's join
	// request is awaiting admin approval (POST /join returns 202 { pending: true }),
	// or seeded from the detail bundle's is_pending flag on load.
	let pendingJoin = $state(untrack(() => initialPending));
	let pending = $state(untrack(() => false));
	let joinError = $state('');
	let followersOpen = $state(false);

	// Re-sync on client-side navigation between spaces — the component
	// persists across [slug] param changes, so without this the previous
	// space's join/follow state bleeds into the new page's buttons.
	$effect(() => {
		space.slug;
		joined = initialJoined;
		pendingJoin = initialPending;
		following = followingProp ?? false;
		pending = false;
		joinError = '';
	});

	// Guest "Join" link — sends the guest to /login with a returnTo so they
	// land back on this space after authenticating.
	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	async function toggleJoin() {
		if (!authed || pending) return;
		const was = joined;
		joined = !was;
		pending = true;
		joinError = '';
		if (was) {
			// Already a member → leave.
			const res = await api.delete<{ left: boolean }>(`/spaces/${space.slug}/leave`);
			if (res.error) {
				joined = was; // rollback
				joinError = res.error.message || 'Failed to leave space. Please try again.';
			} else {
				joined = false;
				pendingJoin = false;
			}
		} else {
			// Not a member → join (or submit a pending request for request-only spaces).
			const res = await api.post<{ joined: boolean; pending?: boolean }>(`/spaces/${space.slug}/join`);
			if (res.error) {
				joined = was; // rollback
				joinError = res.error.message || 'Failed to join space. Please try again.';
			} else if (res.data) {
				// Authoritative state from the server: joined may be false with
				// pending=true for request-only spaces (202 response).
				joined = !!res.data.joined;
				pendingJoin = !!res.data.pending && !res.data.joined;

				// Auto-follow on a successful join (best-effort — a follow
				// failure does not roll back the join). Mirrors the
				// SpaceJoinButton behavior on the /spaces list page.
				// Notifies the parent page so the header Follow button
				// immediately reflects "Following".
				if (joined && !res.data.pending && !following) {
					following = true;
					onFollowChange?.(true);
					void api.post<{ following: boolean }>(`/spaces/${space.slug}/follow`);
				}
			}
		}
		pending = false;
	}
</script>

<div class="space-y-4">
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-start gap-3">
			{#if logoUrl}
				<img
					src={logoUrl}
					alt={space.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-base font-medium text-[var(--color-text-subtle)]"
				>
					{space.name.charAt(0).toUpperCase()}
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">{space.name}</h2>
				<p class="text-xs capitalize text-[var(--color-muted)]">{space.level} space</p>
			</div>
		</div>

		<dl
			class="mt-4 grid grid-flow-col grid-cols-3 grid-rows-2 gap-x-2 gap-y-1 text-center"
		>
			<!-- §05 amendment — primary spaces are follow-only; the Members
			     figure (auto-joins) is meaningless, show Followers only. -->
			{#if !space.is_primary}
				<dt class="text-xs text-[var(--color-muted)]">{t('space.members', 'Members')}</dt>
				<dd class="text-sm font-medium text-[var(--color-text)]">{space.member_count}</dd>
			{/if}
			<dt class="text-xs text-[var(--color-muted)]">{t('space.posts', 'Posts')}</dt>
			<dd class="text-sm font-medium text-[var(--color-text)]">{space.post_count}</dd>
			<dt class="text-xs text-[var(--color-muted)]">{t('space.followers', 'Followers')}</dt>
			<dd class="text-sm font-medium text-[var(--color-text)]">
				<!-- GET /spaces/{id}/followers is OptionalAuth — guests can open
				     the follower list too. -->
				<button
					type="button"
					onclick={() => (followersOpen = true)}
					class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
				>
					{space.follower_count}
				</button>
			</dd>
		</dl>

		<!-- §05 amendment — primary (platform-run) spaces are follow-only:
		     no Join CTA (auto-joined; the platform owns them). The header
		     Follow button remains the action. -->
		{#if !space.is_primary}
		{#if authed}
			{#if joinError}
				<p class="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{joinError}</p>
			{/if}
			<button
				type="button"
				onclick={toggleJoin}
				disabled={pending || (joined && isOwner)}
				aria-busy={pending}
				title={joined && isOwner ? t('space.owner_cannot_leave', 'The sole owner cannot leave — delete the space or ask a platform admin') : undefined}
				class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50 {joined
					? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
					: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
			>
				{#if joined && isOwner}
					<Shield size={16} /> {t('space.owned', 'Owned')}
				{:else if joined}
					<LogIn class="rtl:-scale-x-100" size={16} /> {t('space.joined', 'Joined')}
				{:else if pendingJoin}
					<LogIn class="rtl:-scale-x-100" size={16} /> {t('space.pending', 'Pending')}
				{:else}
					<LogIn class="rtl:-scale-x-100" size={16} /> {t('space.join', 'Join')}
				{/if}
			</button>
		{:else}
			<a
				href={loginHref}
				class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<LogIn class="rtl:-scale-x-100" size={16} /> {t('space.join', 'Join')}
			</a>
		{/if}
		{/if}

		<!-- §05 space tips: member-created custom spaces only — the button
		     fetches tip-settings and self-hides when disabled/ineligible. -->
		{#if space.level === 'custom'}
			<div class="mt-3">
				<TipButton
					recipientType="space"
					recipientKey={space.slug}
					recipientId={space.id}
					recipientName={space.name}
				/>
			</div>
		{/if}
	</section>

	{#if rules.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<ScrollText size={16} /> {t('space.rules', 'Rules')}
			</h2>
			<ol class="space-y-3">
				{#each rules as rule, i (rule.id)}
					<li class="text-sm">
						<p class="font-medium text-[var(--color-text)]">{i + 1}. {rule.title}</p>
						{#if rule.body}
							<p class="mt-0.5 text-[var(--color-text-subtle)]">{rule.body}</p>
						{/if}
					</li>
				{/each}
			</ol>
		</section>
	{/if}

	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<FileText size={16} /> {t('space.about', 'About')}
		</h2>
		{#if space.description}
			<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{space.description}</p>
		{:else if wikiExtract}
			<p class="text-sm leading-relaxed text-[var(--color-text-subtle)] line-clamp-4">{wikiExtract}</p>
		{:else}
			<p class="text-sm text-[var(--color-muted)]">{t('space.no_description', 'No description yet.')}</p>
		{/if}
		{#if wikiExtract && space.description}
			<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)] line-clamp-4">{wikiExtract}</p>
		{/if}
	</section>

	<!-- §89 Community Achievement Feed — sidebar widget showing latest
	     achievements filed against this Space (country). Auto-hides when
	     the feature is disabled or the space has no achievements. -->
	<AchievementWidget spaceSlug={space.slug} spaceId={space.id} {achievements} />

	<!-- New members widget — shows the newest approved members (the members
	     list is ordered joined_at DESC). Hidden when the space has no
	     members yet (audit fix: previously rendered a raw member_count line
	     labelled "New members"). On primary spaces the member set IS the
	     follower set — labelled accordingly (§05 amendment). -->
	{#if members.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Users size={16} /> {space.is_primary ? t('space.new_followers', 'New followers') : t('space.new_members', 'New members')}
			</h2>
			<ul class="space-y-2.5">
				{#each members.slice(0, 5) as member (member.user_id)}
					<li>
						<a href="/u/{member.username}" class="flex items-center gap-2.5 transition-colors hover:text-[var(--color-primary)]">
							{#if member.avatar_url}
								<img src={member.avatar_url} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
									{(member.display_name ?? member.username).charAt(0).toUpperCase()}
								</span>
							{/if}
							<span class="min-w-0">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.display_name ?? member.username}</span>
								<span class="block truncate text-xs text-[var(--color-muted)]">@{member.username}</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
			<p class="mt-3 text-xs text-[var(--color-muted)]">{space.is_primary
				? t('space.follower_count', '{n} followers').replace('{n}', String(space.follower_count))
				: t('space.member_count', '{n} members').replace('{n}', String(space.member_count))}</p>
		</section>
	{/if}

	<SpaceFollowListModal bind:open={followersOpen} spaceId={space.id} spaceName={space.name} count={space.follower_count} />
</div>
