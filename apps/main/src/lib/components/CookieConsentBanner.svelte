<script lang="ts">
	/**
	 * §19 cookie-consent banner — the consent-capture half of GDPR/NDPR
	 * compliance. Shown once to every visitor (guest or member) until a
	 * decision is made, then hidden for a year.
	 *
	 * The jc_consent cookie is the operative record: GuestSessionMiddleware
	 * on the API only mints the jc_guest visitor-id cookie after an
	 * affirmative "all", and expires it under any other decision. Strictly
	 * necessary cookies (jc_token, jc_refresh, csrf_token, jc_locale,
	 * jc_theme) are not gated — they carry no consent requirement.
	 *
	 * The decision is also POSTed to /api/v1/consent as the server-side
	 * audit trail — functional_cookies (the jc_guest gate) plus
	 * analytics_tracking. Anonymous → consent_records.user_id NULL keyed on
	 * IP; authenticated → user-bound rows. Fire-and-forget: the cookie alone
	 * governs what the backend may set.
	 *
	 * Withdrawal: navigating to #cookie-settings clears the decision and
	 * re-shows the banner (footer link "Cookie settings" → href="/#cookie-settings").
	 *
	 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { PUBLIC_JC_API_URL } from '$app/env/public';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { Cookie } from 'lucide-svelte';

	const CONSENT_COOKIE = 'jc_consent';
	const CONSENT_MAX_AGE = 365 * 24 * 60 * 60; // 1 year, matches jc_guest
	const REOPEN_HASH = '#cookie-settings';

	let visible = $state(false);
	let deciding = $state(false);

	function hasDecision(): boolean {
		return document.cookie.split('; ').some((c) => c.startsWith(`${CONSENT_COOKIE}=`));
	}

	// Scope the cookie to the shared base domain so api.<domain> receives it
	// — same derivation as CSRF_COOKIE_DOMAIN in hooks.server.ts, but only
	// when the current host is same-site with the API host. On custom-domain
	// surfaces (academy hosts like japauni.com) a .japa.to attribute would be
	// rejected outright — the cookie must fall back to host-only (api.* is
	// cross-site there anyway, so consent can't reach it regardless).
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

	function writeConsent(value: 'all' | 'necessary' | '') {
		const domain = cookieDomain();
		const secure = location.protocol === 'https:' ? '; Secure' : '';
		const maxAge = value === '' ? 0 : CONSENT_MAX_AGE;
		document.cookie =
			`${CONSENT_COOKIE}=${value}; path=/; max-age=${maxAge}; SameSite=Lax` +
			(domain ? `; domain=${domain}` : '') +
			secure;
	}

	// #cookie-settings clears the stored decision and re-opens the banner —
	// GDPR requires withdrawal to be as easy as consent. Watching page.url.hash
	// covers native anchor clicks AND SvelteKit client-side navigation, which
	// rewrites the URL via pushState and never emits a hashchange event.
	$effect(() => {
		if (page.url.hash === REOPEN_HASH) {
			writeConsent('');
			deciding = false; // reset the debounce — reopened buttons must work
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
		// Drop the reopen hash so a later identical navigation retriggers.
		if (page.url.hash === REOPEN_HASH) {
			void goto(page.url.pathname + page.url.search, { replaceState: true });
		}
		// functional_cookies is the category the decision actually gates
		// (jc_guest); analytics_tracking covers the cookieless-Umami slot.
		for (const type of ['functional_cookies', 'analytics_tracking']) {
			void api.post('/consent', { consent_type: type, granted }, { 'x-silent-error': '1' });
		}
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
					<a href="/cookies" class="text-[var(--color-primary)] hover:underline">
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
