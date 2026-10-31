<script lang="ts">
	// Shared footer for all docs pages, guest or authenticated.
	// Ref: ADR-054 — footer links are driven by the docs_footer menu slug
	// so admins can add/remove/reorder links from the admin panel without
	// touching frontend code.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { env } from '$env/dynamic/public';
	import { resolveMenuHref } from '$lib/utils/safeHtml';
	import type { Menu, MenuItem, SessionUser } from '$lib/types';

	interface Props {
		/** Navigation menu from GET /menus/docs_footer. */
		menu: Menu | null;
		/** Current user so visibility can be filtered correctly. */
		user?: SessionUser | null;
		/** Compact rendering for the right-rail bottom slot. */
		rail?: boolean;
	}

	let { menu, user = null, rail = false }: Props = $props();

	// Root-relative URLs that aren't docs-local (/, /privacy, /signup, …) live
	// on the main app — resolve them against the main origin (ADR-054).
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

	// Guests see 'all' + 'guest'; authenticated users see 'all' + 'authenticated'.
	const footerItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) =>
			item.visibility === 'all' ||
			(user ? item.visibility === 'authenticated' : item.visibility === 'guest')
		)
	);
</script>

<footer class="guest-footer" class:rail>
	<div class="guest-footer-inner">
		<div class="footer-copy">
			&copy; {new Date(page.data.serverNow ?? Date.now()).getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}
		</div>
		{#if footerItems.length > 0}
			<nav class="footer-nav" aria-label={t('landing.footer_nav', 'Footer navigation')}>
				<ul class="footer-list">
					{#each footerItems as item (item.id)}
						{@const href = resolveMenuHref(item.url, mainOrigin)}
						<li>
							<a
								{href}
								class="footer-link"
								target={item.target === '_blank' ? '_blank' : undefined}
								rel={item.target === '_blank' ? 'noopener noreferrer' : undefined}
								data-sveltekit-reload={item.target === '_blank' || href.startsWith('http')
									? undefined
									: /\.[a-z0-9]+$/i.test(href.split('/').pop() ?? '') || undefined}
							>
								{item.label}
							</a>
						</li>
					{/each}
				</ul>
			</nav>
		{/if}
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
		padding: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.footer-copy {
		font-size: 0.8125rem;
		color: var(--color-muted);
	}
	.footer-nav {
		display: flex;
		justify-content: flex-end;
	}
	.footer-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem 1.5rem;
		justify-content: flex-end;
	}
	.footer-link {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		transition: color 0.15s;
	}
	.footer-link:hover {
		color: var(--color-text);
	}
	@media (max-width: 640px) {
		.guest-footer-inner {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.75rem;
		}
		.footer-list {
			justify-content: flex-start;
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
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.375rem;
	}
	.guest-footer.rail .footer-copy {
		font-size: 0.6875rem;
		line-height: 1rem;
		order: 2;
	}
	.guest-footer.rail .footer-nav {
		justify-content: flex-start;
		order: 1;
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
</style>
