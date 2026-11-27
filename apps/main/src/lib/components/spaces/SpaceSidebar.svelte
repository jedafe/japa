<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import {
		Users, ScrollText, LogIn, Shield, MapPin, Tag, CalendarDays,
		Crown, Trophy, Hash
	} from 'lucide-svelte';
	import { Accordion } from 'bits-ui';
	import { api } from '#lib/api/client.js';
	import { decodeBase64Text } from '#lib/tiptap/serialize.js';
	import AchievementWidget from '#lib/components/spaces/AchievementWidget.svelte';
	import TipButton from '#lib/components/tips/TipButton.svelte';
	import SpaceFollowListModal from '#lib/components/spaces/SpaceFollowListModal.svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import type {
		Space, SpaceMember, SpaceRule, Achievement, CalendarEntry,
		SpaceLeader, SpaceContributor
	} from '#lib/types/index.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';

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
		// §05 hub-redesign sidebar cards — all degrade to hidden when empty.
		/** Ancestor/parent spaces — the About card's Location line. */
		parentSpaces?: Space[];
		/** Resolved space-type display name — the About card's Category. */
		typeName?: string;
		/** Upcoming calendar entries for this destination. */
		events?: CalendarEntry[];
		/** Owners ∪ moderators from GET /spaces/{slug}/leaders. */
		leaders?: SpaceLeader[];
		/** Top contributors from GET /spaces/{slug}/contributors. */
		contributors?: SpaceContributor[];
		/** Space keywords — rendered as "Popular topics" pills → /tags/{slug}. */
		keywords?: string[];
	}
	let {
		space, rules = [], authed, logoUrl = '', initialJoined = false,
		initialPending = false, isOwner = false, following: followingProp = false,
		onFollowChange, achievements = [], members = [], parentSpaces = [],
		typeName = '', events = [], leaders = [], contributors = [], keywords = []
	}: Props = $props();

	// About-card meta rows — joined ancestors for Location, type name for
	// Category, created month for Created (mockup: "March 2024").
	const locationLabel = $derived(parentSpaces.map((p) => p.name).join(' · '));
	const createdLabel = $derived(
		space.created_at
			? new Date(space.created_at).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
			: ''
	);

	let following = $state(untrack(() => followingProp ?? false));

	// Master tips.enabled flag — the space tip routes are gated server-side.
	const tipsEnabled = $derived(!!page.data.features?.['tips.enabled']);

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
	<!-- About this space (§05 hub-redesign) — description + Location /
	     Category / Created meta rows + Join/leave + tips. The stats strip
	     lives on the header card now. -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-start gap-3">
			{#if logoUrl}
				<img
					src={logoUrl}
					alt={space.name}
					class="h-12 w-12 shrink-0 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-base font-medium text-[var(--color-text-subtle)]"
				>
					{space.name.charAt(0).toUpperCase()}
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">
					{t('space.about_this', 'About this space')}
				</h2>
				<p class="text-xs capitalize text-[var(--color-muted)]">{space.level} · {space.visibility}</p>
			</div>
		</div>

		{#if space.description}
			<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">{space.description}</p>
		{:else if wikiExtract}
			<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)] line-clamp-4">{wikiExtract}</p>
		{/if}

		<dl class="mt-3 space-y-2 text-sm">
			{#if locationLabel}
				<div class="flex items-center gap-2">
					<MapPin size={14} class="shrink-0 text-[var(--color-muted)]" />
					<dt class="text-[var(--color-muted)]">{t('space.location', 'Location')}</dt>
					<dd class="ml-auto truncate font-medium text-[var(--color-text)]">{locationLabel}</dd>
				</div>
			{/if}
			{#if typeName}
				<div class="flex items-center gap-2">
					<Tag size={14} class="shrink-0 text-[var(--color-muted)]" />
					<dt class="text-[var(--color-muted)]">{t('space.category', 'Category')}</dt>
					<dd class="ml-auto truncate font-medium text-[var(--color-text)]">{typeName}</dd>
				</div>
			{/if}
			{#if createdLabel}
				<div class="flex items-center gap-2">
					<CalendarDays size={14} class="shrink-0 text-[var(--color-muted)]" />
					<dt class="text-[var(--color-muted)]">{t('space.created', 'Created')}</dt>
					<dd class="ml-auto font-medium text-[var(--color-text)]">{createdLabel}</dd>
				</div>
			{/if}
			<div class="flex items-center gap-2">
				<Users size={14} class="shrink-0 text-[var(--color-muted)]" />
				<dt class="text-[var(--color-muted)]">{space.is_primary ? t('space.followers', 'Followers') : t('space.members', 'Members')}</dt>
				<dd class="ml-auto font-medium text-[var(--color-text)]">
					<!-- GET /spaces/{id}/followers is OptionalAuth — guests can open
					     the follower list too. -->
					<button
						type="button"
						onclick={() => (followersOpen = true)}
						class="hover:text-[var(--color-primary-text)]"
					>
						{space.is_primary ? space.follower_count : space.member_count}
					</button>
				</dd>
			</div>
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
		     fetches tip-settings and self-hides when disabled/ineligible. The
		     master tips.enabled flag gates the space routes server-side, so
		     the button must not render (or fetch) when the feature is off. -->
		{#if space.level === 'custom' && tipsEnabled}
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

	<!-- Community rules — collapsible accordion (mockup). -->
	{#if rules.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<ScrollText size={16} /> {t('space.rules', 'Community rules')}
			</h2>
			<Accordion.Root type="single" class="divide-y divide-[var(--color-border)]">
				{#each rules as rule, i (rule.id)}
					<Accordion.Item value={rule.id}>
						<Accordion.Header>
							<Accordion.Trigger class="flex w-full items-center justify-between py-2 text-start text-sm font-medium text-[var(--color-text)] data-[state=open]:text-[var(--color-primary-text)]">
								{i + 1}. {rule.title}
								<span class="text-xs text-[var(--color-muted)] transition-transform group-data-[state=open]:rotate-180">▾</span>
							</Accordion.Trigger>
						</Accordion.Header>
						{#if rule.body}
							<Accordion.Content class="pb-3 text-sm text-[var(--color-text-subtle)]">
								{rule.body}
							</Accordion.Content>
						{/if}
					</Accordion.Item>
				{/each}
			</Accordion.Root>
		</section>
	{/if}

	<!-- Upcoming events (§86) — top 3 from the same /calendar fetch that
	     feeds the Events tab. -->
	{#if events.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<CalendarDays size={16} /> {t('space.events.upcoming', 'Upcoming events')}
			</h2>
			<ul class="space-y-3">
				{#each events.slice(0, 3) as ev (ev.id)}
					{@const dt = new Date(ev.occurs_at)}
					<li class="flex items-center gap-2.5">
						<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
							<span class="text-center leading-tight">
								<span class="block text-[9px] font-semibold uppercase">{dt.toLocaleDateString(undefined, { month: 'short' })}</span>
								<span class="block text-sm font-bold">{dt.getDate()}</span>
							</span>
						</span>
						<span class="min-w-0">
							<span class="block truncate text-sm font-medium text-[var(--color-text)]">{ev.title}</span>
							<span class="block truncate text-xs text-[var(--color-muted)]">
								{dt.toLocaleString(undefined, { weekday: 'short', hour: 'numeric', minute: '2-digit' })}
							</span>
						</span>
					</li>
				{/each}
			</ul>
			<a
				href="/calendar?dest={space.id}"
				class="mt-3 inline-block text-xs font-medium text-[var(--color-primary-text)] hover:underline"
			>
				{t('space.events.view_all', 'View all events')} →
			</a>
		</section>
	{/if}

	<!-- Space leaders — owners ∪ moderators (GET /spaces/{slug}/leaders). -->
	{#if leaders.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Crown size={16} /> {t('space.leaders', 'Space leaders')}
			</h2>
			<ul class="space-y-2.5">
				{#each leaders as leader (leader.user_id)}
					<li class="flex items-center gap-2.5">
						<a href="/u/{leader.username}" class="flex min-w-0 flex-1 items-center gap-2.5 transition-colors hover:text-[var(--color-primary-text)]">
							{#if leader.avatar_url}
								<img src={remoteImageUrl(leader.avatar_url)} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
									{(leader.display_name ?? leader.username).charAt(0).toUpperCase()}
								</span>
							{/if}
							<span class="min-w-0">
								<span class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]"><span class="truncate">{leader.display_name ?? leader.username}</span><VerifiedBadge verified={leader.verified} type={leader.verification_type} size={13} /></span>
								<span class="block truncate text-xs text-[var(--color-muted)]">@{leader.username}</span>
							</span>
						</a>
						<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[10px] font-medium text-[var(--color-muted)]">
							{leader.role === 'owner' ? t('space.role.owner', 'Owner') : t('space.role.moderator', 'Mod')}
						</span>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- Popular topics — space keywords as pills → /tags/{slug}. -->
	{#if keywords.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Hash size={16} /> {t('space.topics', 'Popular topics')}
			</h2>
			<div class="flex flex-wrap gap-1.5">
				{#each keywords as kw (kw)}
					<a
						href="/tags/{encodeURIComponent(kw)}"
						class="rounded-full border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)]"
					>
						#{kw}
					</a>
				{/each}
			</div>
		</section>
	{/if}

	<!-- Top contributors (wiki-tab sidebar card, §05 hub-redesign). -->
	{#if contributors.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Trophy size={16} /> {t('space.contributors', 'Top contributors')}
			</h2>
			<ul class="space-y-2.5">
				{#each contributors as contributor, i (contributor.user_id)}
					<li class="flex items-center gap-2.5">
						<span class="w-4 shrink-0 text-center text-xs font-semibold text-[var(--color-muted)]">{i + 1}</span>
						<a href="/u/{contributor.username}" class="flex min-w-0 flex-1 items-center gap-2.5 transition-colors hover:text-[var(--color-primary-text)]">
							{#if contributor.avatar_url}
								<img src={remoteImageUrl(contributor.avatar_url)} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
									{(contributor.display_name ?? contributor.username).charAt(0).toUpperCase()}
								</span>
							{/if}
							<span class="min-w-0">
								<span class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]"><span class="truncate">{contributor.display_name ?? contributor.username}</span><VerifiedBadge verified={contributor.verified} type={contributor.verification_type} size={13} /></span>
								<span class="block truncate text-xs text-[var(--color-muted)]">
									{t('space.contributions', '{n} contributions').replace('{n}', String(contributor.score))}
								</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

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
						<a href="/u/{member.username}" class="flex items-center gap-2.5 transition-colors hover:text-[var(--color-primary-text)]">
							{#if member.avatar_url}
								<img src={remoteImageUrl(member.avatar_url)} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
									{(member.display_name ?? member.username).charAt(0).toUpperCase()}
								</span>
							{/if}
							<span class="min-w-0">
								<span class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]"><span class="truncate">{member.display_name ?? member.username}</span><VerifiedBadge verified={member.verified} type={member.verification_type} size={13} /></span>
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
