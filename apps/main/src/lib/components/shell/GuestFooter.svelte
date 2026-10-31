<script lang="ts">
	// Shared footer for unauthenticated users across all pages.
	// Extracted from the GuestLanding homepage footer so every guest page
	// (blog, explore, forum, docs, academy, homepage) has the same footer.
	// Ref: ADR-054 — footer links are driven by the main_footer menu slug
	// so admins can add/remove/reorder links from the admin panel without
	// touching frontend code.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import type { Menu, MenuItem } from '$lib/types';
	import { routeFeatureGateVisible } from './routeFeatureFlags';

	interface Props {
		/** Navigation menu from GET /menus/main_footer — filtered to guest-visible items. */
		menu: Menu | null;
		/** Feature flags from /feature-flags — items whose route maps to a
		 *  disabled flag are hidden so gated features never leak to guests. */
		features?: Record<string, boolean>;
		/** Compact rendering for the right-rail bottom slot. */
		rail?: boolean;
	}

	let { menu, features = {}, rail = false }: Props = $props();

	// Filter to guest-visible items only (visibility === 'guest' or 'all'),
	// then drop items whose route maps to a disabled feature flag.
	// The backend already filters by auth state, but this is a safety net for
	// cached/hydrated data and keeps the component self-documenting.
	const footerItems = $derived(
		(menu?.items ?? []).filter(
			(item: MenuItem) =>
				item.visibility !== 'authenticated' && routeFeatureGateVisible(item.url, features)
		)
	);
</script>

<footer class="guest-footer" class:rail>
	<div class="guest-footer-inner">
		{#if footerItems.length > 0}
			<nav class="footer-nav" aria-label={t('landing.footer_nav', 'Footer navigation')}>
				<ul class="footer-list">
					{#each footerItems as item (item.id)}
						<li>
							<a
								href={item.url}
								class="footer-link"
								target={item.target === '_blank' ? '_blank' : undefined}
								rel={item.target === '_blank' ? 'noopener noreferrer' : undefined}
							>
								{item.label}
							</a>
						</li>
					{/each}
				</ul>
			</nav>
		{:else}
			<!-- Fallback: if the menu API is unavailable, show nothing rather than
			     stale hardcoded links. The footer bottom (copyright) still renders. -->
		{/if}
		<div class="footer-bottom">
			&copy; {new Date(page.data.serverNow ?? Date.now()).getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}
		</div>
	</div>
</footer>

<style>
	.guest-footer {
		border-top: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.guest-footer-inner {
		max-width: 72rem;
		margin: 0 auto;
		padding: 3rem 1.5rem;
	}
	.footer-nav {
		display: flex;
		justify-content: center;
	}
	.footer-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 1.5rem;
		justify-content: center;
	}
	.footer-link {
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		transition: color 0.15s;
	}
	.footer-link:hover {
		color: var(--color-text);
	}
	.footer-bottom {
		margin-top: 2.5rem;
		border-top: 1px solid var(--color-border);
		padding-top: 1.5rem;
		text-align: center;
		font-size: 0.875rem;
		color: var(--color-muted);
	}
	@media (min-width: 640px) {
		.footer-list {
			gap: 2rem;
		}
	}

	/* Rail variant — footer pinned to the bottom of the right sidebar:
	   compact, quiet, left-aligned instead of the full-width page footer. */
	.guest-footer.rail {
		border-top: none;
		background: transparent;
	}
	.guest-footer.rail .guest-footer-inner {
		max-width: none;
		margin: 0;
		padding: 0;
	}
	.guest-footer.rail .footer-nav {
		justify-content: flex-start;
	}
	.guest-footer.rail .footer-list {
		column-gap: 0.875rem;
		row-gap: 0.375rem;
		justify-content: flex-start;
	}
	.guest-footer.rail .footer-link {
		font-size: 0.75rem;
		line-height: 1.25rem;
		color: var(--color-muted);
	}
	.guest-footer.rail .footer-bottom {
		margin-top: 0.375rem;
		border-top: none;
		padding-top: 0;
		text-align: left;
		font-size: 0.6875rem;
		line-height: 1rem;
	}
</style>
