<script lang="ts">
	/**
	 * §19 cookie-consent banner — GDPR/NDPR consent capture for the Academy
	 * surface. Mirrors apps/main's CookieConsentBanner (per-app copies, like
	 * PwaInstallBanner); the jc_consent cookie written on .<base-domain> is
	 * read by the API's GuestSessionMiddleware for all surfaces.
	 *
	 * Withdrawal: #cookie-settings clears the decision and re-shows.
	 *
	 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { PUBLIC_JC_API_URL } from '$app/env/public';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { readConsent, setConsent, clearConsent, initConsent } from '#lib/consent.js';
	import { Cookie } from 'lucide-svelte';
	import { Switch } from 'bits-ui';

	const CONSENT_COOKIE = 'jc_consent';
	const REOPEN_HASH = '#cookie-settings';

	let visible = $state(false);
	let deciding = $state(false);
	// "Customize" view — per-category switches, seeded from the previous
	// decision when the banner is re-opened via #cookie-settings.
	let customizing = $state(false);
	let functional = $state(false);
	let analytics = $state(false);

	function hasDecision(): boolean {
		return document.cookie.split('; ').some((c) => c.startsWith(`${CONSENT_COOKIE}=`));
	}

	// The policy page lives on the main surface; derive its host from the
	// shared API base domain, falling back to a relative path in dev.
	function policyUrl(): string {
		try {
			const apiHost = new URL(PUBLIC_JC_API_URL ?? '').hostname;
			const base = apiHost.startsWith('api.') ? apiHost.slice(4) : apiHost;
			return `https://${base}/cookies`;
		} catch {
			return '/cookies';
		}
	}

	// Watching page.url.hash covers native anchor clicks AND SvelteKit
	// client-side navigation (pushState never emits a hashchange event).
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
		if (!visible) visible = !hasDecision();
	});

	// Record a per-category decision: jc_consent encodes the granted types,
	// and each consent_type gets its own audit row so a partial choice
	// (e.g. analytics without functional) is represented honestly.
	function decide(f: boolean, a: boolean) {
		if (deciding) return;
		deciding = true;
		setConsent(f, a);
		visible = false;
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
						<a href={policyUrl()} class="text-[var(--color-primary)] hover:underline">
							{t('cookies.banner.learn_more', 'Learn more')}
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
						<a href={policyUrl()} class="text-[var(--color-primary)] hover:underline">
							{t('cookies.banner.learn_more', 'Learn more')}
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
