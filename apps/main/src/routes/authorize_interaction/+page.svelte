<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import { Globe, AlertCircle } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let handle = $state('');
	let error = $state('');

	// Parse the visitor's handle to its home domain. Accepts the common
	// forms: user@instance, @user@instance, acct:user@instance, or a
	// profile URL (https://instance/@user).
	function homeDomain(input: string): string {
		let h = input.trim();
		if (h.startsWith('acct:')) h = h.slice(5);
		if (h.startsWith('@')) h = h.slice(1);
		if (h.includes('://')) {
			try {
				return new URL(h).hostname.toLowerCase();
			} catch {
				return '';
			}
		}
		if (h.includes('/')) {
			// Scheme-less profile URL — 'mastodon.social/@user' is a common
			// paste form; anchor the scheme and read the hostname.
			try {
				return new URL('https://' + h).hostname.toLowerCase();
			} catch {
				return '';
			}
		}
		const at = h.lastIndexOf('@');
		if (at < 0) return '';
		const domain = h.slice(at + 1).toLowerCase();
		return /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/.test(domain) && domain.includes('.') ? domain : '';
	}

	function follow() {
		const domain = homeDomain(handle);
		if (!domain) {
			error = t('federation.bad_handle', 'Enter a full fediverse handle like you@mastodon.social');
			return;
		}
		const target = `https://${domain}/authorize_interaction?uri=${encodeURIComponent(data.uri)}`;
		window.location.href = target;
	}
</script>

<SeoHead title={t('federation.remote_follow_title', 'Remote follow — {site}')} robots="noindex" description={t('federation.remote_follow_description', 'Follow this fediverse actor from your home instance.')} />

<div class="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center px-4">
	{#if !data.uri}
		<div class="w-full text-center">
			<AlertCircle size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('federation.remote_follow_title', 'Remote follow')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('federation.missing_uri', 'This link is missing the actor to follow. Open it from a fediverse app, or paste an actor address into your instance’s search.')}
			</p>
			<a href="/" class="mt-4 inline-block text-sm text-[var(--color-primary-text)] hover:underline">
				{t('common.back_home', 'Back to home')}
			</a>
		</div>
	{:else}
		<div class="w-full text-center">
			<Globe size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('federation.remote_follow_title', 'Remote follow')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('federation.remote_follow_body', 'To follow {target} from your own fediverse instance, enter your handle below.', { target: data.uri })}
			</p>
			{#if error}
				<div class="mt-3 flex items-center justify-center gap-2 text-sm text-red-500">
					<AlertCircle size={16} /> {error}
				</div>
			{/if}
			<form
				onsubmit={(e) => {
					e.preventDefault();
					follow();
				}}
				class="mt-4 space-y-3"
			>
				<input
					type="text"
					name="handle"
					bind:value={handle}
					required
					autocomplete="off"
					placeholder={t('federation.handle_placeholder', 'you@your-instance.social')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<button
					type="submit"
					class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90"
				>
					{t('federation.continue_to_instance', 'Continue to my instance')}
				</button>
			</form>
			<p class="mt-3 text-xs text-[var(--color-muted)]">
				{t('federation.remote_follow_hint', 'You will be sent to your own instance to confirm the follow — nothing is shared with us.')}
			</p>
		</div>
	{/if}
</div>
