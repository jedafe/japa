<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import {
		User,
		LayoutDashboard,
		Settings,
		LogOut,
		LogIn,
		Moon,
		Sun
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { env } from '$env/dynamic/public';
	import { page } from '$app/state';
	import type { SessionUser } from '$lib/types';

	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

	interface Props {
		user: SessionUser | null;
	}

	let { user }: Props = $props();

	// Recompute the redirect URL on client-side navigation so login/signup
	// return links point to the current docs page.
	let returnTo = $derived(encodeURIComponent(page.url.href));

	let initials = $derived(
		user?.displayName
			? user.displayName
					.split(' ')
					.map((p) => p[0])
					.slice(0, 2)
					.join('')
					.toUpperCase()
			: ''
	);

	const itemClass =
		'flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]';
</script>

<div class="dropdown">
	<!-- Theme toggle (separate from the avatar dropdown). -->
	<button
		type="button"
		class="toggle"
		onclick={() => theme.toggle()}
		aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
		title={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
	>
		{#if theme.current === 'dark'}
			<Sun size={16} />
		{:else}
			<Moon size={16} />
		{/if}
	</button>

	<!-- Avatar dropdown menu (Bits UI — portals to body, keyboard accessible). -->
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class="grid h-9 w-9 shrink-0 place-items-center rounded-full overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)] cursor-pointer hover:outline hover:outline-2 hover:outline-[var(--color-border)] hover:outline-offset-1"
			aria-label={t('nav.account', 'Account')}
		>
			{#if user?.avatar_url}
				<img src={user.avatar_url} alt={user.displayName} class="h-full w-full object-cover" />
			{:else if user}
				<span class="grid h-full w-full place-items-center text-xs font-semibold text-[var(--color-primary-contrast)] bg-[var(--color-primary)]">{initials}</span>
			{:else}
				<User size={18} />
			{/if}
		</DropdownMenu.Trigger>
		<DropdownMenu.Portal>
			<DropdownMenu.Content
				align="end"
				sideOffset={8}
				class="z-50 min-w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
			>
				{#if user}
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
							<a href={`${mainOrigin}/u/${user.username}`} {...props}>
								<User size={16} /> {t('nav.profile', 'Profile')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
							<a href={`${mainOrigin}/dashboard`} {...props}>
								<LayoutDashboard size={16} /> {t('nav.dashboard', 'Dashboard')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
							<a href={`${mainOrigin}/settings/profile`} {...props}>
								<Settings size={16} /> {t('nav.settings', 'Settings')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
					<DropdownMenu.Separator class="my-1 h-px bg-[var(--color-border)]" />
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
							<a href={`${mainOrigin}/logout`} {...props}>
								<LogOut size={16} /> {t('nav.logout', 'Logout')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
				{:else}
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
						<a href={`${mainOrigin}/login?redirect=${returnTo}`} {...props}>
							<LogIn size={16} /> {t('auth.login')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
					<DropdownMenu.Item class={itemClass}>
						{#snippet child({ props })}
						<a href={`${mainOrigin}/signup?redirect=${returnTo}`} {...props}>
							<User size={16} /> {t('auth.signup')}
							</a>
						{/snippet}
					</DropdownMenu.Item>
				{/if}
			</DropdownMenu.Content>
		</DropdownMenu.Portal>
	</DropdownMenu.Root>
</div>

<style>
	.dropdown {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}
	.toggle {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border-radius: var(--radius);
		color: var(--color-muted);
		background: transparent;
		border: 1px solid var(--color-border);
		cursor: pointer;
	}
	.toggle:hover {
		color: var(--color-text);
		border-color: var(--color-border-input-hover);
	}
</style>
