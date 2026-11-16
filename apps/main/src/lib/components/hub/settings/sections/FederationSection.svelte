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

	type FederationStatus = {
		enabled?: boolean;
		status?: string;
		actor_url?: string;
		handle?: string;
		follower_count?: number;
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

	let { data }: { data: unknown } = $props();
	const sectionData = $derived(
		data as { status: FederationStatus | null; requests: RemoteFollowRequest[] } | null ?? null
	);
	const fed = $derived(sectionData?.status ?? null);
	const requests = $derived(sectionData?.requests ?? []);

	let busy = $state(false);
	let busyRequest = $state<string | null>(null);
	let message = $state<{ kind: 'success' | 'error'; text: string } | null>(null);
	let disableOpen = $state(false);

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
				</dl>
			</li>
		</ul>

		{#if fed.enabled && requests.length > 0}
			<div class="mt-4">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.federation.requests_title', 'Follow requests')}</h2>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">{t('settings.federation.requests_desc', 'Remote accounts waiting for your approval before they can follow you.')}</p>
				<ul class="mt-2 space-y-2">
					{#each requests as req (req.remote_actor_id)}
						<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5">
							{#if req.avatar_url}
								<img src={req.avatar_url} alt="" class="size-8 shrink-0 rounded-full object-cover" loading="lazy" />
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
									<p class="truncate text-xs text-[var(--color-muted)]">{req.acct}</p>
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

		<div class="mt-3 flex items-center justify-end gap-2">
			{#if fed.enabled}
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
