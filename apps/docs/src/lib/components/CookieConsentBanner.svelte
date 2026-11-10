<script lang="ts">
	/**
	 * §19 cookie-consent banner — GDPR/NDPR consent capture for the Docs
	 * surface. Mirrors apps/main's CookieConsentBanner (per-app copies, like
	 * PwaInstallBanner); the jc_consent cookie written on .<base-domain> is
	 * read by the API's GuestSessionMiddleware for all surfaces.
	 *
	 * The consent POST goes straight to the API origin — /api/authed proxies
	 * only whitelisted prefixes, and guests have no Bearer to attach anyway.
	 * Withdrawal: #cookie-settings clears the decision and re-shows.
	 *
	 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { PUBLIC_JC_API_URL } from '$app/env/public';
	import { buildApiUrl } from '@japaconnect/api-url';
	import { t } from '#lib/i18n/index.js';
	import { Cookie } from 'lucide-svelte';

	const CONSENT_COOKIE = 'jc_consent';
	const CONSENT_MAX_AGE = 365 * 24 * 60 * 60; // 1 year, matches jc_guest
	const REOPEN_HASH = '#cookie-settings';
	const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

	let visible = $state(false);
	let deciding = $state(false);

	function hasDecision(): boolean {
		return document.cookie.split('; ').some((c) => c.startsWith(`${CONSENT_COOKIE}=`));
	}

	// Scope the cookie to the shared base domain so api.<domain> receives it,
	// but only when the current host is same-site with the API host.
	function cookieDomain(): string {
		try {
			const apiHost = new URL(PUBLIC_JC_API_URL ?? '').hostname;
			if (!apiHost.startsWith('api.')) return '';
			const base = apiHost.slice(4);
			const host = location.hostname;
			return host === base || host.endsWith(`.${base}`) ? `.${base}` : '';
		} catch {
			return '';
		}
	}

	// The policy page lives on the main surface; derive its host from the
	// shared API base domain, falling back to a relative path in dev.
	function policyUrl(): string {
		try {
			const apiHost = new URL(PUBLIC_JC_API_URL ?? '').hostname;
			const base = apiHost.startsWith('api.') ? apiHost.slice(4) : apiHost;
			return `https://${base}/pages/privacy`;
		} catch {
			return '/pages/privacy';
		}
	}

	function writeConsent(value: 'all' | 'necessary' | '') {
		const domain = cookieDomain();
		const secure = location.protocol === 'https:' ? '; Secure' : '';
		const maxAge = value === '' ? 0 : CONSENT_MAX_AGE;
		document.cookie =
			`${CONSENT_COOKIE}=${value}; path=/; max-age=${maxAge}; SameSite=Lax` +
			(domain ? `; domain=${domain}` : '') +
			secure;
	}

	// Watching page.url.hash covers native anchor clicks AND SvelteKit
	// client-side navigation (pushState never emits a hashchange event).
	$effect(() => {
		if (page.url.hash === REOPEN_HASH) {
			writeConsent('');
			visible = true;
		}
	});

	onMount(() => {
		if (!visible) visible = !hasDecision();
	});

	function decide(granted: boolean) {
		if (deciding) return;
		deciding = true;
		writeConsent(granted ? 'all' : 'necessary');
		visible = false;
		if (page.url.hash === REOPEN_HASH) {
			void goto(page.url.pathname + page.url.search, { replaceState: true });
		}
		// Fire-and-forget audit trail — POST /consent is public, and the CSRF
		// exemption is scoped to requests without a session credential.
		void fetch(buildApiUrl('/consent', API_BASE), {
			method: 'POST',
			credentials: 'include',
			headers: { 'Content-Type': 'application/json', 'X-JC-Surface': 'docs' },
			body: JSON.stringify({ consent_type: 'analytics_tracking', granted })
		}).catch(() => {});
	}
</script>

{#if visible}
	<div
		class="fixed inset-x-0 bottom-0 z-[150] flex justify-center p-3 sm:p-4"
		role="dialog"
		aria-label={t('cookies.banner.aria', 'Cookie consent')}
	>
		<div
			class="flex w-full max-w-2xl flex-col gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg sm:flex-row sm:items-center"
		>
			<div class="flex min-w-0 flex-1 items-start gap-2.5">
				<Cookie size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
				<p class="text-sm leading-snug text-[var(--color-muted)]">
					{t(
						'cookies.banner.body',
						'We use cookies to keep you signed in and remember your preferences, plus optional analytics cookies that help us improve the platform.'
					)}
					<a href={policyUrl()} class="text-[var(--color-primary)] hover:underline">
						{t('cookies.banner.learn_more', 'Learn more')}
					</a>
				</p>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				<button
					type="button"
					onclick={() => decide(false)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('cookies.banner.necessary_only', 'Necessary only')}
				</button>
				<button
					type="button"
					onclick={() => decide(true)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('cookies.banner.accept_all', 'Accept all')}
				</button>
			</div>
		</div>
	</div>
{/if}
