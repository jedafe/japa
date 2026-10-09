<script lang="ts">
	// §106 video — channel claim / takedown form.
	// Authenticated users can claim a synthetic channel or request a takedown
	// of a specific video source on a channel.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { ShieldQuestion, Loader2, Send, FileCheck, Video } from 'lucide-svelte';
	import { listMyVideoClaims, submitChannelClaim } from '$lib/api/videos';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import type { PublicUser, VideoChannelClaim, VideoSource } from '$lib/types';

	const username = $derived(page.params.username);
	const currentUser = $derived(page.data.user as PublicUser | null | undefined);
	const isAuthed = $derived(!!currentUser);

	let target = $state<PublicUser | null>(null);
	let videos = $state<VideoSource[]>([]);
	let myClaims = $state<VideoChannelClaim[]>([]);
	let loading = $state(true);
	let loadError = $state('');

	let claimType = $state<'claim' | 'takedown'>('claim');
	let evidenceUrl = $state('');
	let notes = $state('');
	let selectedVideoId = $state('');

	let submitting = $state(false);
	let submitSuccess = $state(false);
	let submitError = $state('');

	const isSynthetic = $derived(!!target?.is_synthetic);

	$effect(() => {
		// Track both auth and username so navigation from /u/foo/claim to
		// /u/bar/claim re-fetches the new target.
		if (!isAuthed || !username) {
			loading = false;
			return;
		}
		loadTarget();
	});

	async function loadTarget() {
		loading = true;
		loadError = '';
		const [userRes, videosRes, claimsRes] = await Promise.all([
			api.get<PublicUser>(`/users/${username}`),
			// GET /u/{username}/videos returns a bare array in `data` (not an
			// {items} wrapper) — read it directly.
			api.get<VideoSource[]>(`/u/${username}/videos?limit=100`),
			listMyVideoClaims()
		]);
		loading = false;
		if (userRes.data) {
			target = userRes.data;
		} else if (userRes.error) {
			loadError = userRes.error.message;
		} else {
			loadError = t('video_claims.load_failed', 'Failed to load profile');
		}
		if (videosRes.data) {
			videos = videosRes.data ?? [];
		}
		myClaims = claimsRes.data ?? [];
		// Default to claim when the target is synthetic; otherwise takedown.
		if (target && !target.is_synthetic) {
			claimType = 'takedown';
		}
	}

	function validate(): boolean {
		submitError = '';
		if (claimType === 'takedown') {
			if (!selectedVideoId) {
				submitError = t('video_claims.errors.select_video', 'Please select a video to request a takedown for.');
				return false;
			}
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!isAuthed || !target) return;
		if (!validate()) return;

		submitting = true;
		submitSuccess = false;
		try {
			const body: {
				claim_type: 'claim' | 'takedown';
				evidence_url?: string;
				notes?: string;
				video_source_id?: string;
			} = {
				claim_type: claimType,
				evidence_url: evidenceUrl || undefined,
				notes: notes || undefined
			};
			if (claimType === 'takedown') {
				body.video_source_id = selectedVideoId;
			}
			const res = await submitChannelClaim(displayUsername, body);
			if (res.error) {
				submitError = res.error.message;
			} else {
				submitSuccess = true;
				evidenceUrl = '';
				notes = '';
				selectedVideoId = '';
				if (res.data) myClaims = [...myClaims, res.data];
			}
		} catch {
			submitError = t('video_claims.errors.submit_failed', 'Submission failed. Please try again.');
		} finally {
			submitting = false;
		}
	}

	const displayUsername = $derived(username ?? '');
	// §106 — the caller's existing requests against this channel (GET
	// /me/video-claims), so a second pending claim can't be filed blindly.
	const channelClaims = $derived.by(() => {
		const authorId = target?.id;
		return authorId ? myClaims.filter((c) => c.video_author_user_id === authorId) : [];
	});
	const claimStatusLabel = (status: string) =>
		t(`video_claims.status.${status}`, status);
	const title = $derived(t('video_claims.title', 'Claim or report channel — {username}').replace('{username}', displayUsername ?? ''));
	const pageDescription = $derived(
		t('video_claims.description', 'Submit a claim or takedown request for this channel.')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-3 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<ShieldQuestion size={20} />
			{t('video_claims.heading', 'Claim or report channel')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{pageDescription}
		</p>
	</div>

	<div class="p-3">
		{#if !isAuthed}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
				<p class="text-sm text-[var(--color-text)]">
					{t('video_claims.sign_in_required', 'Sign in to submit a claim or takedown request.')}
				</p>
			</div>
		{:else if loading}
			<div class="flex items-center justify-center gap-2 py-8 text-[var(--color-muted)]">
				<Loader2 size={20} class="animate-spin" />
				<span class="text-sm">{t('common.loading', 'Loading…')}</span>
			</div>
		{:else if loadError}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{loadError}
			</p>
		{:else if !target}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
				<p class="text-sm text-[var(--color-text)]">
					{t('video_claims.user_not_found', 'Channel not found.')}
				</p>
			</div>
		{:else if submitSuccess}
			<div class="rounded-[var(--radius-card)] border border-green-500/30 bg-green-500/10 p-6 text-center">
				<FileCheck size={32} class="mx-auto mb-2 text-green-600" />
				<p class="text-lg font-medium text-green-600">
					{t('video_claims.submitted', 'Request submitted')}
				</p>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t('video_claims.submitted_body', 'An admin will review your request.')}
				</p>
				<a
					href="/u/{username}"
					class="mt-4 inline-block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					← {t('video_claims.back_to_channel', 'Back to channel')}
				</a>
			</div>
		{:else}
			<div
				class="mb-4 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<span
					class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Video size={18} />
				</span>
				<div class="min-w-0">
					<p class="truncate text-sm font-medium text-[var(--color-text)]">
						{target.display_name || target.username}
					</p>
					<p class="truncate text-xs text-[var(--color-muted)]">@{target.username}</p>
				</div>
			</div>

			{#if submitError}
				<p class="mb-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{submitError}
				</p>
			{/if}

			{#if channelClaims.length > 0}
				<div
					class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<p class="mb-2 text-sm font-medium text-[var(--color-text)]">
						{t('video_claims.existing_heading', 'Your requests for this channel')}
					</p>
					<ul class="space-y-2">
						{#each channelClaims as claim (claim.id)}
							<li class="flex items-center justify-between gap-3 text-sm">
								<span class="text-[var(--color-text)]">
									{claim.claim_type === 'takedown'
										? t('video_claims.type.takedown', 'Request a takedown')
										: t('video_claims.type.claim', 'Claim this channel')}
								</span>
								<span class="flex shrink-0 items-center gap-2">
									<span
										class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[11px] font-medium capitalize text-[var(--color-text-subtle)]"
									>
										{claimStatusLabel(claim.status)}
									</span>
									{#if claim.status === 'rejected'}
										<!-- §22: rejected channel claims are appealable. -->
										<AppealButton entityType="video_channel_claim" entityId={claim.id} class="!px-2 !py-0.5 text-[11px]" />
									{/if}
								</span>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<form onsubmit={submit} class="space-y-4">
				<fieldset class="space-y-2">
					<legend class="text-sm font-medium text-[var(--color-text)]">
						{t('video_claims.type_label', 'Request type')}
					</legend>
					<div class="flex flex-wrap gap-4">
						<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input
								type="radio"
								name="claim_type"
								value="claim"
								bind:group={claimType}
								disabled={!isSynthetic}
								class="accent-[var(--color-primary)]"
							/>
							{t('video_claims.type.claim', 'Claim this channel')}
						</label>
						{#if !isSynthetic}
							<span class="text-xs text-[var(--color-muted)]">
								{t('video_claims.claim_not_available', 'Only synthetic channels can be claimed.')}
							</span>
						{/if}
						<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input
								type="radio"
								name="claim_type"
								value="takedown"
								bind:group={claimType}
								class="accent-[var(--color-primary)]"
							/>
							{t('video_claims.type.takedown', 'Request a takedown')}
						</label>
					</div>
				</fieldset>

				{#if claimType === 'takedown'}
					<div class="space-y-1">
						<label for="video-source" class="text-sm font-medium text-[var(--color-text)]">
							{t('video_claims.video_label', 'Video to remove')}
						</label>
						<AutocompleteSelect
							id="video-source"
							bind:value={selectedVideoId}
							options={[
								{ value: '', label: t('video_claims.select_video', 'Select a video…') },
								...videos.map((v) => ({ value: v.id, label: v.title || v.source_url }))
							]}
						/>
					</div>
				{/if}

				<div class="space-y-1">
					<label for="evidence-url" class="text-sm font-medium text-[var(--color-text)]">
						{t('video_claims.evidence_url', 'Evidence URL')}
					</label>
					<input
						id="evidence-url"
						type="url"
						bind:value={evidenceUrl}
						placeholder={t('video_claims.evidence_placeholder', 'https://example.com/evidence')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					/>
				</div>

				<div class="space-y-1">
					<label for="notes" class="text-sm font-medium text-[var(--color-text)]">
						{t('video_claims.notes', 'Notes')}
					</label>
					<textarea
						id="notes"
						bind:value={notes}
						rows={4}
						placeholder={t('video_claims.notes_placeholder', 'Explain why you are submitting this request…')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
				</div>

				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Send size={16} />
					{/if}
					{t('video_claims.submit', 'Submit request')}
				</button>
			</form>
		{/if}
	</div>
</div>
