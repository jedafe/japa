<script lang="ts">
	// Shared footer for unauthenticated users across all docs pages.
	// Ref: ADR-054 — footer links are driven by the docs_footer menu slug
	// so admins can add/remove/reorder links from the admin panel without
	// touching frontend code.
	import { t } from '$lib/i18n';
	import type { Menu, MenuItem } from '$lib/types';

	interface Props {
		/** Navigation menu from GET /menus/docs_footer — filtered to guest-visible items. */
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
		{/if}
		<div class="footer-bottom">
			&copy; {new Date().getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}
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
</style>
