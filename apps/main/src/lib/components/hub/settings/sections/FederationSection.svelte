<script lang="ts">
	/**
	 * Fediverse opt-in/opt-out — Section 126 / ADR-138.
	 *
	 * Renders the member's ActivityPub identity fetched from
	 * GET /me/federation (Rule 16: no hardcoded data) and toggles it via
	 * POST (enable) / DELETE (disable) /me/federation. When no actor row
	 * exists (status 'none') an enable CTA is shown — needed whenever
	 * federation.auto_optin is off; when auto-opt-in already provisioned
	 * the actor, this is the member's opt-out surface.
	 *
	 * Ref: japaconnect-engineering/sections/126-activitypub-federation.md
	 *      japaconnect-engineering/adr/ADR-138-activitypub-federation.md
	 */
	import { Modal } from '#lib/components/ui/index.js';
	import { invalidateAll } from '$app/navigation';
	import { api } from '#lib/api/client.js';
	import { t } from '#lib/i18n/index.js';
	import { AtSign, Users } from 'lucide-svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';

	type FederationStatus = {
		enabled?: boolean;
		status?: string;
		actor_url?: string;
		handle?: string;
		follower_count?: number;
		moved_to_url?: string | null;
		created_at?: string;
	};

	type RemoteFollowRequest = {
		remote_actor_id: string;
		actor_url?: string | null;
		acct?: string | null;
		display_name?: string | null;
		avatar_url?: string | null;
		created_at?: string;
	};

	// §126 M5 — one outbound edge: pending until the remote inbox Accepts.
	type RemoteFollow = RemoteFollowRequest & {
		preferred_username?: string | null;
		status?: string;
	};

	let { data }: { data: unknown } = $props();
	const sectionData = $derived(
		data as {
			status: FederationStatus | null;
			requests: RemoteFollowRequest[];
			following?: RemoteFollow[];
		} | null ?? null
	);
	const fed = $derived(sectionData?.status ?? null);
	const requests = $derived(sectionData?.requests ?? []);
	const following = $derived(sectionData?.following ?? []);

	let busy = $state(false);
	let busyRequest = $state<string | null>(null);
	let busyFollow = $state<string | null>(null);
	let followTarget = $state('');
	let moveTarget = $state('');
	let message = $state<{ kind: 'success' | 'error'; text: string } | null>(null);
	let disableOpen = $state(false);
	let rotateOpen = $state(false);
	let moveOpen = $state(false);

	async function handleEnable() {
		if (busy) return;
		busy = true;
		message = null;
		const res = await api.post<FederationStatus>('/me/federation');
		busy = false;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		message = { kind: 'success', text: t('settings.federation.enabled_msg', 'Federation is now enabled for your account.') };
		await invalidateAll();
	}

	async function handleResolve(req: RemoteFollowRequest, accept: boolean) {
		if (busyRequest) return;
		busyRequest = req.remote_actor_id;
		message = null;
		const res = await api.post<{ status: string }>(
			`/me/federation/follow-requests/${req.remote_actor_id}/${accept ? 'accept' : 'reject'}`
		);
		busyRequest = null;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		message = {
			kind: 'success',
			text: accept
				? t('settings.federation.request_accepted', 'Follow request accepted.')
				: t('settings.federation.request_rejected', 'Follow request rejected.')
		};
		await invalidateAll();
	}

	async function handleDisable() {
		if (busy) return;
		busy = true;
		message = null;
		const res = await api.delete<FederationStatus>('/me/federation');
		busy = false;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		disableOpen = false;
		message = { kind: 'success', text: t('settings.federation.disabled_msg', 'Federation is now disabled. Remote servers will no longer see your actor.') };
		await invalidateAll();
	}

	// §126 M5 — outbound follows: the member pastes an acct (user@domain)
	// or actor URL; the backend WebFingers/fetches it and delivers a
	// signed Follow. Accepted edges feed the Global home tab timeline.
	async function handleFollow() {
		const target = followTarget.trim();
		if (busy || !target) return;
		busy = true;
		message = null;
		const res = await api.post<RemoteFollow>('/me/federation/following', { target });
		busy = false;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		followTarget = '';
		message = {
			kind: 'success',
			text: res.data?.status === 'accepted'
				? t('settings.federation.follow_accepted_msg', 'Now following — their public posts will appear in your Global feed.')
				: t('settings.federation.follow_pending_msg', 'Follow request sent. Their posts will appear in your Global feed once the remote server accepts.')
		};
		await invalidateAll();
	}

	async function handleUnfollow(f: RemoteFollow) {
		if (busyFollow) return;
		busyFollow = f.remote_actor_id;
		message = null;
		const res = await api.delete(`/me/federation/following/${f.remote_actor_id}`);
		busyFollow = null;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		message = { kind: 'success', text: t('settings.federation.unfollowed_msg', 'Unfollowed.') };
		await invalidateAll();
	}

	// §126 M5 — re-drive a pending outbound edge: re-ships the same
	// signed Follow when the remote's Accept/Reject never arrived.
	async function handleResend(f: RemoteFollow) {
		if (busyFollow || !f.actor_url) return;
		busyFollow = f.remote_actor_id;
		message = null;
		const res = await api.post<RemoteFollow>('/me/federation/following', { target: f.actor_url });
		busyFollow = null;
		if (res.error) {
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		message = { kind: 'success', text: t('settings.federation.follow_resent_msg', 'Follow request re-sent to the remote server.') };
		await invalidateAll();
	}

	// §126 M5 — outbound Move: stamps movedTo on the actor document and
	// fans a signed Move to remote followers. The target account must
	// already list this actor in its alsoKnownAs — the backend rejects
	// unconfirmed moves and cools repeat moves per
	// federation.move.cooldown_days.
	async function handleMove() {
		const target = moveTarget.trim();
		if (busy || !target) return;
		busy = true;
		message = null;
		const res = await api.post('/me/federation/move', { target_actor_url: target });
		busy = false;
		if (res.error) {
			moveOpen = false;
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		moveOpen = false;
		moveTarget = '';
		message = { kind: 'success', text: t('settings.federation.moved_msg', 'Account moved. Remote followers have been notified and your actor now redirects to the new account.') };
		await invalidateAll();
	}

	// §126 M3a — signing-key rotation: replaces the actor's keypair and
	// announces the new key to remote followers (Update{Person}).
	async function handleRotate() {
		if (busy) return;
		busy = true;
		message = null;
		const res = await api.post<{ key_id: string; rotated_at: string }>('/me/federation/rotate-key');
		busy = false;
		if (res.error) {
			rotateOpen = false;
			message = { kind: 'error', text: res.error.message ?? t('settings.federation.error', 'Something went wrong. Please try again.') };
			return;
		}
		rotateOpen = false;
		message = { kind: 'success', text: t('settings.federation.rotated_msg', 'Signing key rotated. Remote servers have been notified.') };
		await invalidateAll();
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.federation', 'Fediverse')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.federation.desc', 'Share your public profile and published content with the fediverse (Mastodon and other ActivityPub platforms).')}</p>
	</header>

	{#if fed && fed.status !== 'none'}
		<ul class="space-y-2">
			<li class="flex flex-col gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-3">
				<div class="flex flex-1 items-start gap-3">
					<AtSign size={18} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
					<div class="min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)]">
							{fed.enabled ? t('settings.federation.status_active', 'Federation is on') : t('settings.federation.status_disabled', 'Federation is off')}
						</p>
						<p class="mt-1 text-xs text-[var(--color-muted)]">
							{#if fed.enabled}
								{t('settings.federation.active_desc', 'Your public profile and published public content are visible to fediverse servers.')}
							{:else}
								{t('settings.federation.disabled_desc', 'Your actor is deactivated — remote servers can no longer fetch it. You can re-enable at any time.')}
							{/if}
						</p>
					</div>
				</div>
				<dl class="mt-1 space-y-1 border-t border-[var(--color-border)] pt-2 text-xs">
					{#if fed.handle}
						<div class="flex items-center justify-between gap-3">
							<dt class="text-[var(--color-muted)]">{t('settings.federation.handle', 'Fediverse handle')}</dt>
							<dd class="font-mono text-[var(--color-text)]">{fed.handle}</dd>
						</div>
					{/if}
					<div class="flex items-center justify-between gap-3">
						<dt class="text-[var(--color-muted)]">{t('settings.federation.followers', 'Fediverse followers')}</dt>
						<dd class="flex items-center gap-1 text-[var(--color-text)]"><Users size={12} class="text-[var(--color-muted)]" />{fed.follower_count ?? 0}</dd>
					</div>
					{#if fed.actor_url}
						<div class="flex items-center justify-between gap-3">
							<dt class="text-[var(--color-muted)]">{t('settings.federation.actor_url', 'Actor URL')}</dt>
							<dd class="min-w-0 truncate font-mono text-[var(--color-text)]" title={fed.actor_url}>{fed.actor_url}</dd>
						</div>
					{/if}
					{#if fed.moved_to_url}
						<div class="flex items-center justify-between gap-3">
							<dt class="text-[var(--color-muted)]">{t('settings.federation.moved_to', 'Moved to')}</dt>
							<dd class="min-w-0 truncate font-mono text-[var(--color-text)]" title={fed.moved_to_url}>{fed.moved_to_url}</dd>
						</div>
					{/if}
				</dl>
			</li>
		</ul>

		{#if fed.moved_to_url}
			<p class="mt-2 rounded-[var(--radius)] border border-[var(--color-warning,#f59e0b)]/40 bg-[var(--color-warning,#f59e0b)]/10 px-3 py-2 text-xs text-[var(--color-text)]">
				{t('settings.federation.moved_banner', 'This account has migrated. Remote servers now redirect your actor to the new account above.')}
			</p>
		{/if}

		{#if fed.enabled && requests.length > 0}
			<div class="mt-4">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.federation.requests_title', 'Follow requests')}</h2>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">{t('settings.federation.requests_desc', 'Remote accounts waiting for your approval before they can follow you.')}</p>
				<ul class="mt-2 space-y-2">
					{#each requests as req (req.remote_actor_id)}
						<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5">
							{#if req.avatar_url}
								<img src={remoteImageUrl(req.avatar_url)} alt="" class="size-8 shrink-0 rounded-full object-cover" loading="lazy" />
							{:else}
								<span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-subtle)]">
									<AtSign size={14} class="text-[var(--color-muted)]" />
								</span>
							{/if}
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">
									{req.display_name || req.acct || req.actor_url}
								</p>
								{#if req.acct}
									<p class="truncate text-xs text-[var(--color-muted)]">@{req.acct}</p>
								{/if}
							</div>
							<div class="flex shrink-0 items-center gap-1.5">
								<button
									type="button"
									disabled={busyRequest === req.remote_actor_id}
									onclick={() => handleResolve(req, true)}
									class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-white disabled:opacity-50"
								>
									{t('settings.federation.request_accept', 'Accept')}
								</button>
								<button
									type="button"
									disabled={busyRequest === req.remote_actor_id}
									onclick={() => handleResolve(req, false)}
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>
									{t('settings.federation.request_reject', 'Reject')}
								</button>
							</div>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if fed.enabled}
			<!-- §126 M5 — remote follows: accounts the member follows out to
			     other servers. Accepted edges feed the Global home tab. -->
			<div class="mt-4">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.federation.following_title', 'Remote accounts you follow')}</h2>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">
					{t('settings.federation.following_desc', 'Follow fediverse accounts and their public posts appear in your Global feed on the home tab.')}
					<a href="/?tab=global" class="font-medium text-[var(--color-primary)] hover:underline">{t('settings.federation.open_global', 'Open Global feed')}</a>
				</p>
				<form
					class="mt-2 flex items-center gap-2"
					onsubmit={(e) => { e.preventDefault(); handleFollow(); }}
				>
					<input
						type="text"
						bind:value={followTarget}
						placeholder={t('settings.federation.follow_placeholder', 'user@server.social or https://server.social/users/name')}
						class="min-w-0 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)]"
					/>
					<button
						type="submit"
						disabled={busy || !followTarget.trim()}
						class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					>
						{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.follow_action', 'Follow')}
					</button>
				</form>
				{#if following.length > 0}
					<ul class="mt-2 space-y-2">
						{#each following as f (f.remote_actor_id)}
							<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5">
								{#if f.avatar_url}
									<img src={remoteImageUrl(f.avatar_url)} alt="" class="size-8 shrink-0 rounded-full object-cover" loading="lazy" />
								{:else}
									<span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-subtle)]">
										<AtSign size={14} class="text-[var(--color-muted)]" />
									</span>
								{/if}
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium text-[var(--color-text)]">
										{f.display_name || f.acct || f.actor_url}
									</p>
									{#if f.acct}
										<p class="truncate text-xs text-[var(--color-muted)]">@{f.acct}</p>
									{/if}
								</div>
								<span class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]">
									{f.status === 'accepted'
										? t('settings.federation.follow_status_accepted', 'Following')
										: t('settings.federation.follow_status_pending', 'Pending')}
								</span>
								{#if f.status === 'pending'}
									<button
										type="button"
										disabled={busyFollow === f.remote_actor_id}
										onclick={() => handleResend(f)}
										title={t('settings.federation.follow_resend_hint', 'The remote server has not answered yet — send the follow request again.')}
										class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
									>
										{t('settings.federation.follow_resend', 'Resend')}
									</button>
								{/if}
								<button
									type="button"
									disabled={busyFollow === f.remote_actor_id}
									onclick={() => handleUnfollow(f)}
									class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>
									{f.status === 'pending'
										? t('settings.federation.follow_withdraw', 'Withdraw')
										: t('settings.federation.unfollow', 'Unfollow')}
								</button>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="mt-2 text-xs text-[var(--color-muted)]">{t('settings.federation.following_empty', 'You are not following any remote accounts yet.')}</p>
				{/if}
			</div>

			<!-- §126 M5 — outbound Move: migrate this fediverse identity to
			     an account on another server. The target must already list
			     this actor in its alsoKnownAs (their setup step); our signed
			     Move then redirects remote followers. -->
			{#if !fed.moved_to_url}
				<div class="mt-4">
					<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.federation.migrate_title', 'Move to another server')}</h2>
					<p class="mt-0.5 text-xs text-[var(--color-muted)]">{t('settings.federation.migrate_desc', 'Redirect your fediverse identity to an account on another ActivityPub server. Remote followers are notified automatically.')}</p>
					<button
						type="button"
						onclick={() => (moveOpen = true)}
						class="mt-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('settings.federation.migrate_action', 'Move account…')}
					</button>
				</div>
			{/if}
		{/if}

		<div class="mt-3 flex items-center justify-end gap-3">
			{#if fed.enabled}
				<button
					type="button"
					onclick={() => (rotateOpen = true)}
					class="text-xs font-medium text-[var(--color-muted)] hover:text-[var(--color-text)] hover:underline"
				>
					{t('settings.federation.rotate_key', 'Rotate signing key')}
				</button>
				<button
					type="button"
					onclick={() => (disableOpen = true)}
					class="text-xs font-medium text-[var(--color-danger,#dc2626)] hover:underline"
				>
					{t('settings.federation.disable', 'Disable federation')}
				</button>
			{:else}
				<button
					type="button"
					disabled={busy}
					onclick={handleEnable}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
				>
					{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.enable', 'Enable federation')}
				</button>
			{/if}
		</div>

		<Modal bind:open={disableOpen} title={t('settings.federation.disable_confirm_title', 'Disable federation?')} description={t('settings.federation.disable_confirm_desc', 'Your fediverse actor stops serving immediately and remote servers will no longer see your profile or posts. You can re-enable at any time — your handle is preserved.')} size="sm">
			{#snippet children(close)}
				<div class="flex items-center justify-end gap-2">
					<button
						type="button"
						disabled={busy}
						onclick={close}
						class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('settings.action.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						disabled={busy}
						onclick={handleDisable}
						class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					>
						{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.disable', 'Disable federation')}
					</button>
				</div>
			{/snippet}
		</Modal>

		<Modal bind:open={rotateOpen} title={t('settings.federation.rotate_confirm_title', 'Rotate signing key?')} description={t('settings.federation.rotate_confirm_desc', 'Your actor gets a fresh RSA keypair and remote servers are notified (Update activity). Deliveries in flight begin signing with the new key. Rotation is rate-limited — only do this if you suspect your key is compromised.')} size="sm">
			{#snippet children(close)}
				<div class="flex items-center justify-end gap-2">
					<button
						type="button"
						disabled={busy}
						onclick={close}
						class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('settings.action.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						disabled={busy}
						onclick={handleRotate}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					>
						{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.rotate_key', 'Rotate signing key')}
					</button>
				</div>
			{/snippet}
		</Modal>

		<Modal bind:open={moveOpen} title={t('settings.federation.migrate_confirm_title', 'Move to another server?')} description={t('settings.federation.migrate_confirm_desc', 'Your fediverse actor will redirect to the new account and remote followers will be sent a signed Move notice. On the target server you must first add this account’s handle to your new account’s aliases (alsoKnownAs) or the move is rejected. This cannot be undone from here.')} size="sm">
			{#snippet children(close)}
				<form
					class="space-y-3"
					onsubmit={(e) => { e.preventDefault(); handleMove(); }}
				>
					<input
						type="text"
						bind:value={moveTarget}
						placeholder={t('settings.federation.migrate_placeholder', 'https://server.social/users/you')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)]"
					/>
					<div class="flex items-center justify-end gap-2">
						<button
							type="button"
							disabled={busy}
							onclick={close}
							class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							{t('settings.action.cancel', 'Cancel')}
						</button>
						<button
							type="submit"
							disabled={busy || !moveTarget.trim()}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
						>
							{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.migrate_action', 'Move account…')}
						</button>
					</div>
				</form>
			{/snippet}
		</Modal>
	{:else}
		<!-- No actor row: enable CTA (explicit opt-in path) -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
			<AtSign size={32} class="mx-auto text-[var(--color-muted)]" />
			<h2 class="mt-3 text-sm font-medium text-[var(--color-text)]">{t('settings.federation.not_enabled', 'Federation is not enabled for your account')}</h2>
			<p class="mt-1 text-xs text-[var(--color-muted)]">{t('settings.federation.not_enabled_desc', 'Enable it to get a fediverse handle (you@instance) that remote servers can follow, and to share your public posts beyond this site.')}</p>
			<button
				type="button"
				disabled={busy}
				onclick={handleEnable}
				class="mt-4 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{busy ? t('common.submitting', 'Submitting…') : t('settings.federation.enable', 'Enable federation')}
			</button>
		</div>
	{/if}

	{#if message}
		<p class="mt-3 text-sm {message.kind === 'error' ? 'text-[var(--color-danger)]' : 'text-[var(--color-success)]'}">{message.text}</p>
	{/if}
</section>
