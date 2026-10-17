<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';
	import type { SessionUser } from '$lib/types';

	/**
	 * HubHeader — the simplified JapaConnect header for the tools hub shell.
	 * No community navigation (no spaces, forum, or feed links) — just the
	 * logo, a link back to the main app, theme toggle, and auth links.
	 * Ref: §78 (Tools Web Surface — hub shell), ADR-086.
	 */
	let {
		user,
		logoUrl = '',
		logoDarkUrl = ''
	}: {
		user: SessionUser | null;
		logoUrl?: string;
		logoDarkUrl?: string;
	} = $props();
</script>

<header
	class="sticky top-0 z-50 h-[var(--header-height)] bg-surface/80 backdrop-blur-md border-b border-border"
>
	<div class="mx-auto max-w-[var(--max-width)] h-full flex items-center justify-between px-4">
		<div class="flex items-center gap-3">
			<a href="/" class="flex items-center gap-2 font-bold text-text text-lg">
				{#if logoUrl}
					<ThemeLogo {logoUrl} {logoDarkUrl} alt="JapaConnect" class="h-7 w-auto" />
				{:else}
					<span class="flex items-center gap-1.5">
						<span
							class="flex items-center justify-center w-7 h-7 rounded-md bg-primary text-primary-contrast text-sm font-bold"
						>
							J
						</span>
						<span>JapaConnect <span class="text-muted font-normal">{t('app.tagline', 'Tools')}</span></span>
					</span>
				{/if}
			</a>
		</div>

		<div class="flex items-center gap-2">
			<a
				href={mainAppUrl('/')}
				class="hidden sm:inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-text-subtle hover:text-text transition-colors"
			>
				<Icon name="arrow-left" size={16} />
				{t('shell.back_to_japaconnect', 'Back to JapaConnect')}
			</a>

			<button
				onclick={() => theme.toggle()}
				class="p-2 rounded-lg text-muted hover:text-text hover:bg-surface-subtle transition-colors"
				aria-label={t('shell.toggle_theme', 'Toggle theme')}
			>
				<Icon name={theme.resolved === 'dark' ? 'sun' : 'moon'} size={18} />
			</button>

			{#if user}
				<a
					href={mainAppUrl(`/u/${user.username}`)}
					class="flex items-center gap-2 px-2 py-1 rounded-lg hover:bg-surface-subtle transition-colors"
				>
					{#if user.avatar_url}
						<img
							src={user.avatar_url}
							alt={user.displayName}
							class="w-7 h-7 rounded-full object-cover"
						/>
					{:else}
						<div
							class="flex items-center justify-center w-7 h-7 rounded-full bg-primary text-primary-contrast text-xs font-semibold"
						>
							{user.displayName.charAt(0).toUpperCase()}
						</div>
					{/if}
					<span class="hidden sm:inline text-sm font-medium text-text">{user.displayName}</span>
				</a>
			{:else}
				<a
					href={mainAppUrl('/login')}
					class="inline-flex items-center px-3 py-1.5 text-sm font-medium text-primary hover:text-primary-hover transition-colors"
				>
					{t('shell.sign_in', 'Sign in')}
				</a>
			{/if}
		</div>
	</div>
</header>
