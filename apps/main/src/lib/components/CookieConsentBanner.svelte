<script lang="ts">
	/**
	 * §19 cookie-consent banner — the consent-capture half of GDPR/NDPR
	 * compliance. Shown once to every visitor (guest or member) until a
	 * decision is made, then hidden for a year.
	 *
	 * The jc_consent cookie is the operative record — 'all', 'necessary',
	 * or a '+'-joined list of granted consent_types. GuestSessionMiddleware
	 * mints the jc_guest visitor-id cookie only when functional_cookies is
	 * granted, and expires it otherwise; the layouts inject Umami only when
	 * analytics_tracking is granted. Strictly necessary cookies (jc_token,
	 * jc_refresh, csrf_token, jc_locale, jc_theme) are never gated.
	 *
	 * Each category decision is also POSTed to /api/v1/consent as its own
	 * audit row so partial Customize choices record honestly. Anonymous →
	 * consent_records.user_id NULL keyed on IP; authenticated → user-bound
	 * rows. Fire-and-forget: the cookie alone governs what the backend may
	 * set.
	 *
	 * Withdrawal: navigating to #cookie-settings clears the decision and
	 * re-shows the banner (footer link "Cookie settings" → href="/#cookie-settings").
	 *
	 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import {
		readConsent,
		setConsent,
		clearConsent,
		initConsent,
		hasConsentDecision
	} from '#lib/consent.svelte.js';
	import { Cookie } from 'lucide-svelte';
	import { Switch } from 'bits-ui';

	const REOPEN_HASH = '#cookie-settings';

	let visible = $state(false);
	let deciding = $state(false);
	// "Customize" view — per-category switches, seeded from the previous
	// decision when the banner is re-opened via #cookie-settings.
	let customizing = $state(false);
	let functional = $state(false);
	let analytics = $state(false);

	// #cookie-settings clears the stored decision and re-opens the banner —
	// GDPR requires withdrawal to be as easy as consent. Watching page.url.hash
	// covers native anchor clicks AND SvelteKit client-side navigation, which
	// rewrites the URL via pushState and never emits a hashchange event.
	// The previous choice seeds the toggles so the panel reflects the
	// decision being changed, not a blank slate.
	$effect(() => {
		if (page.url.hash === REOPEN_HASH) {
			const prev = readConsent();
			functional = prev.functional;
			analytics = prev.analytics;
			clearConsent();
			deciding = false; // reset the debounce — reopened buttons must work
			customizing = true;
			visible = true;
		}
	});

	onMount(() => {
		initConsent();
		if (!visible) visible = !hasConsentDecision();
	});

	// Record a per-category decision: jc_consent encodes the granted types,
	// and each consent_type gets its own audit row so a partial choice
	// (e.g. analytics without functional) is represented honestly.
	function decide(f: boolean, a: boolean) {
		if (deciding) return;
		deciding = true;
		setConsent(f, a);
		visible = false;
		// Drop the reopen hash so a later identical navigation retriggers.
		if (page.url.hash === REOPEN_HASH) {
			void goto(page.url.pathname + page.url.search, { replaceState: true });
		}
		const decisions: Array<[string, boolean]> = [
			['functional_cookies', f],
			['analytics_tracking', a]
		];
		for (const [type, granted] of decisions) {
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
		{#if customizing}
			<!-- Per-category manager: strictly necessary is always on (not
			     consentable); functional/analytics are opt-in switches. -->
			<div
				class="flex w-full max-w-2xl flex-col gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg"
			>
				<div class="flex items-start gap-2.5">
					<Cookie size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
					<p class="text-sm leading-snug text-[var(--color-muted)]">
						{t('cookies.banner.customize_body', 'Choose which optional cookies we may use.')}
						<a href="/cookies" class="text-[var(--color-primary)] hover:underline">
							{t('cookies.banner.learn_more', 'Learn more about cookies')}
						</a>
					</p>
				</div>
				<ul class="space-y-2">
					<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2">
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium text-[var(--color-text)]">
								{t('cookies.banner.necessary_title', 'Strictly necessary')}
							</span>
							<span class="block text-xs text-[var(--color-muted)]">
								{t('cookies.banner.necessary_desc', 'Sign-in and security. Always on.')}
							</span>
						</span>
						<span class="shrink-0 text-xs font-medium text-[var(--color-muted)]">
							{t('cookies.banner.always_on', 'Always on')}
						</span>
					</li>
					<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2">
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium text-[var(--color-text)]">
								{t('cookies.banner.functional_title', 'Functional cookies')}
							</span>
							<span class="block text-xs text-[var(--color-muted)]">
								{t('cookies.banner.functional_desc', 'Remembers your visitor session so tools and journeys work across visits.')}
							</span>
						</span>
						<Switch.Root
							checked={functional}
							aria-label={t('cookies.banner.functional_title', 'Functional cookies')}
							onCheckedChange={(v) => (functional = v)}
							class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-[var(--color-primary)] data-[state=unchecked]:bg-[var(--color-border)]"
						>
							<Switch.Thumb
								class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5"
							/>
						</Switch.Root>
					</li>
					<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2">
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium text-[var(--color-text)]">
								{t('cookies.banner.analytics_title', 'Analytics')}
							</span>
							<span class="block text-xs text-[var(--color-muted)]">
								{t('cookies.banner.analytics_desc', 'Cookieless, privacy-first usage statistics.')}
							</span>
						</span>
						<Switch.Root
							checked={analytics}
							aria-label={t('cookies.banner.analytics_title', 'Analytics')}
							onCheckedChange={(v) => (analytics = v)}
							class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-[var(--color-primary)] data-[state=unchecked]:bg-[var(--color-border)]"
						>
							<Switch.Thumb
								class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5"
							/>
						</Switch.Root>
					</li>
				</ul>
				<div class="flex flex-wrap items-center justify-end gap-2">
					<button
						type="button"
						onclick={() => decide(true, true)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('cookies.banner.accept_all', 'Accept all')}
					</button>
					<button
						type="button"
						onclick={() => decide(functional, analytics)}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
					>
						{t('cookies.banner.save_preferences', 'Save preferences')}
					</button>
				</div>
			</div>
		{:else}
			<div
				class="flex w-full max-w-2xl flex-col gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg sm:flex-row sm:items-center"
			>
				<div class="flex min-w-0 flex-1 items-start gap-2.5">
					<Cookie size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
					<p class="text-sm leading-snug text-[var(--color-muted)]">
						{t(
							'cookies.banner.body',
							'We use cookies to keep you signed in and remember your preferences, plus optional analytics that help us improve the platform.'
						)}
						<a href="/cookies" class="text-[var(--color-primary)] hover:underline">
							{t('cookies.banner.learn_more', 'Learn more about cookies')}
						</a>
					</p>
				</div>
				<div class="flex shrink-0 items-center gap-2">
					<button
						type="button"
						onclick={() => (customizing = true)}
						class="rounded-[var(--radius)] px-3 py-2 text-sm font-medium text-[var(--color-muted)] hover:text-[var(--color-text)]"
					>
						{t('cookies.banner.customize', 'Customize')}
					</button>
					<button
						type="button"
						onclick={() => decide(false, false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('cookies.banner.necessary_only', 'Necessary only')}
					</button>
					<button
						type="button"
						onclick={() => decide(true, true)}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
					>
						{t('cookies.banner.accept_all', 'Accept all')}
					</button>
				</div>
			</div>
		{/if}
	</div>
{/if}
