<script lang="ts">
	// §124 — install prompt banner: shown once the browser fires
	// beforeinstallprompt (Chromium) or for manual iOS guidance. Dismissal is
	// remembered in a cookie so the banner doesn't nag every session; it
	// never renders once the app runs standalone.
	// Ref: .jeos/sections/124-progressive-web-app.md
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { pwaInstall } from '@japaconnect/ui/pwa-install';
	import { X, Download, Share } from 'lucide-svelte';

	const DISMISS_COOKIE = 'jc_pwa_banner';

	// Admin-uploaded 192px icon (pwa.icon_192) wins over the bundled default.
	const iconUrl = $derived(
		(page.data.siteSettings?.['pwa.icon_192'] as string | undefined) || '/icon-192.png'
	);

	let dismissed = $state(false);
	let installing = $state(false);

	const visible = $derived(
		!dismissed && !pwaInstall.installed && (pwaInstall.canPrompt || pwaInstall.needsManualIos)
	);

	onMount(() => {
		dismissed = document.cookie.includes(`${DISMISS_COOKIE}=1`);
	});

	function dismiss() {
		dismissed = true;
		// Cookie (not localStorage) — UI-state persistence follows the
		// jc_theme/jc_locale cookie convention.
		document.cookie = `${DISMISS_COOKIE}=1; path=/; max-age=${60 * 60 * 24 * 90}; samesite=lax`;
	}

	async function install() {
		installing = true;
		try {
			// A dismissed native dialog counts as a dismissal too — remember it
			// so the banner doesn't re-nag next session.
			const accepted = await pwaInstall.prompt();
			if (!accepted) dismiss();
		} finally {
			installing = false;
		}
	}
</script>

{#if visible}
	<div
		class="fixed inset-x-3 z-50 mx-auto flex max-w-md items-center gap-3 rounded-[var(--radius-card,12px)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 shadow-lg"
		style="bottom: max(1.5rem, calc(env(safe-area-inset-bottom) + 0.75rem))"
		role="dialog"
		aria-label={t('pwa.install_title', 'Install app')}
	>
		<img src={iconUrl} alt="" class="h-10 w-10 shrink-0 rounded-[10px]" />
		<div class="min-w-0 flex-1">
			<p class="text-sm font-medium text-[var(--color-text)]">{t('pwa.install_title', 'Install app')}</p>
			{#if pwaInstall.needsManualIos}
				<p class="mt-0.5 flex items-center gap-1 text-xs text-[var(--color-muted)]">
					{t('pwa.install_ios_hint', 'Tap')}
					<Share size={12} class="inline" />
					{t('pwa.install_ios_hint2', 'then "Add to Home Screen"')}
				</p>
			{:else}
				<p class="mt-0.5 truncate text-xs text-[var(--color-muted)]">
					{t('pwa.install_body', 'Get faster access to the documentation.')}
				</p>
			{/if}
		</div>
		{#if pwaInstall.canPrompt}
			<button
				type="button"
				disabled={installing}
				class="flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
				onclick={install}
			>
				<Download size={14} />
				{t('pwa.install_button', 'Install')}
			</button>
		{/if}
		<button
			type="button"
			class="shrink-0 rounded-full p-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
			aria-label={t('pwa.dismiss', 'Dismiss')}
			onclick={dismiss}
		>
			<X size={16} />
		</button>
	</div>
{/if}
