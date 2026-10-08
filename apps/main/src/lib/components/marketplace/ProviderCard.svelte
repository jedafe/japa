<script lang="ts">
	// Provider card — the org (or individual author) behind the listing.
	// Org listings link to /org/{slug}; author-only listings show the
	// author name + avatar without a link (ServiceListing carries no
	// author_username, so a profile URL can't be built safely).
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { ArrowRight, BadgeCheck, Building2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { ServiceListing } from '$lib/types';

	interface Props {
		service: ServiceListing;
	}
	let { service }: Props = $props();
</script>

{#if service.org_name || service.author_name}
	<section
		aria-labelledby="provider-heading"
		class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
	>
		<h2
			id="provider-heading"
			class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
		>
			{t('services.provider_heading', 'About the provider')}
		</h2>
		<div class="mt-3 flex items-center gap-3">
			{#if service.org_name}
				<span
					class="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)]"
				>
					<Building2 size={20} class="text-[var(--color-text-subtle)]" aria-hidden="true" />
				</span>
			{:else if service.author_avatar}
				<img
					src={service.author_avatar}
					alt=""
					class="h-11 w-11 shrink-0 rounded-full object-cover"
				/>
			{/if}
			<div class="min-w-0">
				<p class="flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
					<span class="truncate">{service.org_name ?? service.author_name}</span>
					{#if service.is_verified}
						<BadgeCheck
							size={15}
							class="shrink-0 text-[var(--color-primary)]"
							aria-label={t('services.verified', 'Verified')}
						/>
					{/if}
				</p>
				{#if service.is_verified && service.verification_type}
					<p class="text-xs text-[var(--color-muted)]">
						{t('services.verified', 'Verified')} · {service.verification_type}
					</p>
				{/if}
			</div>
		</div>
		{#if service.org_slug}
			<a
				href="/org/{service.org_slug}"
				class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('services.provider_view', 'View provider')}
				<ArrowRight size={14} aria-hidden="true" />
			</a>
		{/if}
	</section>
{/if}
