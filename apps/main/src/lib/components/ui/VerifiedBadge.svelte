<script lang="ts">
	/**
	 * VerifiedBadge — §07 verification badge next to an entity name. Renders the
	 * type's custom artwork (badge_url from /verification-requests/types) when
	 * set, falling back to the generic BadgeCheck. Renders nothing when
	 * `verified` is falsy.
	 */
	import { BadgeCheck } from 'lucide-svelte';
	import { verificationBadgeUrl, verificationBadgeLabel } from '$lib/stores/verification-badges.svelte';
	import { t } from '$lib/i18n';

	interface Props {
		verified?: boolean;
		type?: string | null;
		scope?: 'user' | 'organization' | 'service_listing';
		size?: number;
		class?: string;
	}
	let { verified = false, type, scope = 'user', size = 18, class: className = '' }: Props = $props();

	const url = $derived(verificationBadgeUrl(scope, type));
	const label = $derived(verificationBadgeLabel(scope, type) ?? t('profile.verified', 'Verified'));
</script>

{#if verified}
	{#if url}
		<img
			src={url}
			alt={label}
			title={label}
			class="shrink-0 {className}"
			style="width:{size}px;height:{size}px"
		/>
	{:else}
		<BadgeCheck {size} class="shrink-0 text-[var(--color-primary-text)] {className}" />
	{/if}
{/if}
