<script lang="ts">
	// Marketplace chrome footer — link nav driven by the admin-managed
	// main_footer menu (same source as GuestFooter, ADR-054) plus a
	// newsletter signup that posts to the existing /newsletter/subscribe
	// endpoint (Section 62) behind the feature.newsletter flag.
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Loader2, Send } from 'lucide-svelte';
	import type { Menu, MenuItem } from '$lib/types';

	interface Props {
		/** Navigation menu from GET /menus/main_footer — filtered to guest-visible items. */
		menu?: Menu | null;
		/** feature.newsletter flag — hides the signup form when the section is off. */
		newsletterEnabled?: boolean;
	}

	let { menu = null, newsletterEnabled = false }: Props = $props();

	// Filter to guest-visible items only (visibility === 'guest' or 'all') —
	// same safety net as GuestFooter for cached/hydrated data.
	const footerItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => item.visibility !== 'authenticated')
	);

	const siteName = $derived(
		((page.data.siteSettings as Record<string, unknown> | undefined)?.['general.site_name'] as
			| string
			| undefined) ?? t('app.name')
	);

	// Newsletter signup — POST /newsletter/subscribe, same contract as the
	// sidebar newsletter_signup widget (SidebarWidgetRenderer).
	let newsletterEmail = $state('');
	let newsletterSubmitting = $state(false);
	let newsletterMessage = $state<{ kind: 'success' | 'error'; text: string } | null>(null);

	async function handleNewsletterSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (newsletterSubmitting) return;
		const email = newsletterEmail.trim();
		if (!email || !email.includes('@') || !email.includes('.')) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_invalid', 'Please enter a valid email.') };
			return;
		}
		newsletterSubmitting = true;
		newsletterMessage = null;
		const res = await api.post<{ status?: string }>('/newsletter/subscribe', { email });
		newsletterSubmitting = false;
		if (res.error) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_error', 'Subscription failed. Please try again.') };
			return;
		}
		newsletterMessage = { kind: 'success', text: t('sidebar.newsletter_success', 'Thanks for subscribing!') };
		newsletterEmail = '';
	}
</script>

<footer class="border-t border-[var(--color-border)] bg-[var(--color-surface)]">
	<div class="mx-auto max-w-6xl px-6 py-12">
		<div class="grid gap-10 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
			<!-- Brand + tagline -->
			<div>
				<p class="text-sm font-semibold text-[var(--color-text)]">{t('marketplace.footer_title', 'JapaConnect Services')}</p>
				<p class="mt-2 max-w-sm text-sm text-[var(--color-text-subtle)]">
					{t('marketplace.footer_tagline', 'Verified diaspora-relevant services and travel packages — find providers who understand your journey.')}
				</p>

				{#if newsletterEnabled}
					<form class="mt-5 max-w-sm" onsubmit={handleNewsletterSubmit}>
						<label for="marketplace-newsletter" class="mb-1.5 block text-xs font-medium text-[var(--color-text-subtle)]">
							{t('marketplace.newsletter_label', 'Get marketplace updates')}
						</label>
						<div class="flex gap-2">
							<input
								id="marketplace-newsletter"
								type="email"
								bind:value={newsletterEmail}
								disabled={newsletterSubmitting}
								placeholder={t('landing.newsletter_placeholder', 'your@email.com')}
								class="h-9 min-w-0 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
							/>
							<button
								type="submit"
								disabled={newsletterSubmitting}
								class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
							>
								{#if newsletterSubmitting}<Loader2 size={14} class="motion-safe:animate-spin" />{:else}<Send size={14} />{/if}
								{t('landing.newsletter_subscribe', 'Subscribe')}
							</button>
						</div>
						{#if newsletterMessage}
							<p
								class="mt-2 text-xs {newsletterMessage.kind === 'error' ? 'text-[var(--color-danger)]' : 'text-[var(--color-success)]'}"
								role="status"
							>
								{newsletterMessage.text}
							</p>
						{/if}
					</form>
				{/if}
			</div>

			<!-- Menu links — admin-managed via /menus/main_footer -->
			{#if footerItems.length > 0}
				<nav aria-label={t('landing.footer_nav', 'Footer navigation')}>
					<ul class="flex flex-wrap gap-x-6 gap-y-2 md:justify-end">
						{#each footerItems as item (item.id)}
							<li>
								<a
									href={item.url}
									class="text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
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
		</div>

		<div class="mt-10 border-t border-[var(--color-border)] pt-6 text-center text-sm text-[var(--color-muted)]">
			&copy; {new Date().getFullYear()} {siteName}. {t('landing.footer_rights', 'All rights reserved.')}
		</div>
	</div>
</footer>
