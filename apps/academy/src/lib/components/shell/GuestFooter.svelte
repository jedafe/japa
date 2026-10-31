<script lang="ts">
	// Shared footer for unauthenticated users across all Academy pages.
	// Ref: ADR-054 — footer links are driven by the academy_footer menu slug
	// so admins can add/remove/reorder links from the admin panel without
	// touching frontend code.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { resolveMenuUrl } from '$lib/appOrigins';
	import type { Menu, MenuItem } from '$lib/types';

	interface Props {
		/** Navigation menu from GET /menus/academy_footer — filtered to guest-visible items. */
		menu: Menu | null;
	}

	let { menu }: Props = $props();

	// Filter to guest-visible items only (visibility === 'guest' or 'all').
	const footerItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => item.visibility !== 'authenticated')
	);
</script>

<footer class="guest-footer">
	<div class="guest-footer-inner">
		{#if footerItems.length > 0}
			<nav class="footer-nav" aria-label={t('landing.footer_nav', 'Footer navigation')}>
				<ul class="footer-list">
					{#each footerItems as item (item.id)}
						<li>
							<a
								href={resolveMenuUrl(item.url)}
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
		{/if}
		<div class="footer-bottom">
			&copy; {new Date(page.data.serverNow ?? Date.now()).getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}
		</div>
	</div>
</footer>

<style>
	/* Rail variant — the footer lives pinned to the bottom of the right
	   sidebar, so it renders compact and quiet instead of a full-width
	   page footer. */
	.guest-footer {
		background: transparent;
	}
	.guest-footer-inner {
		padding: 0;
	}
	.footer-nav {
		display: block;
	}
	.footer-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		column-gap: 0.875rem;
		row-gap: 0.375rem;
	}
	.footer-link {
		font-size: 0.75rem;
		line-height: 1.25rem;
		color: var(--color-muted);
		transition: color 0.15s;
	}
	.footer-link:hover {
		color: var(--color-text);
	}
	.footer-bottom {
		margin-top: 0.375rem;
		font-size: 0.6875rem;
		line-height: 1rem;
		color: var(--color-muted);
	}
</style>
