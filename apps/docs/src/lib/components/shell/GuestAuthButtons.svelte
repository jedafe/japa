<script lang="ts">
	import { LogIn, UserPlus } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { env } from '$env/dynamic/public';
	import { page } from '$app/state';

	interface Props {
		/** Compact variant for the mobile top bar (icon only). */
		compact?: boolean;
	}
	let { compact = false }: Props = $props();

	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
	const returnTo = encodeURIComponent(page.url.href);
</script>

<div class="guest-auth" class:compact>
	<a href={`${mainOrigin}/login?redirect=${returnTo}`} class="guest-auth-login">
		<LogIn class="rtl:-scale-x-100" size={16} />
		{#if !compact}<span>{t('auth.login')}</span>{/if}
	</a>
	<a href={`${mainOrigin}/signup?redirect=${returnTo}`} class="guest-auth-signup">
		<UserPlus size={16} />
		{#if !compact}<span>{t('auth.signup')}</span>{/if}
	</a>
</div>

<style>
	.guest-auth { display: flex; align-items: center; gap: 0.5rem; flex-shrink: 0; }
	.guest-auth-login {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.375rem 0.75rem; border-radius: var(--radius);
		font-size: 0.875rem; font-weight: 500; color: var(--color-text);
		transition: background 0.15s;
	}
	.guest-auth-login:hover { background: var(--color-surface-subtle); }
	.guest-auth-signup {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.375rem 0.75rem; border-radius: var(--radius);
		background: var(--color-primary); color: var(--color-primary-contrast);
		font-size: 0.875rem; font-weight: 600; transition: background 0.15s;
	}
	.guest-auth-signup:hover { background: var(--color-primary-hover); }
	.compact { gap: 0.375rem; }
	.compact .guest-auth-login,
	.compact .guest-auth-signup { padding: 0.375rem; }
</style>
