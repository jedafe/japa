<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import {
		User,
		LayoutDashboard,
		Bookmark,
		Wallet,
		Settings,
		LogOut,
		LogIn
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
	}

	let { user }: Props = $props();

	const itemClass =
		'flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]';
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)] hover:ring-2 hover:ring-[var(--color-border)]"
		aria-label={t('nav.account', 'Account')}
	>
		{#if user?.avatar_url}
			<img src={user.avatar_url} alt={`Avatar of ${user.display_name ?? user.username ?? "user"}`} class="h-9 w-9 rounded-full object-cover" />
		{:else if user}
			{user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
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
						<a href="/u/{user.username}" {...props}>
							<User size={16} /> {t('nav.profile', 'Profile')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/dashboard" {...props}>
							<LayoutDashboard size={16} /> {t('nav.dashboard', 'Dashboard')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/dashboard/bookmarks" {...props}>
							<Bookmark size={16} /> {t('nav.bookmarks', 'Bookmarks')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/settings/wallet" {...props}>
							<Wallet size={16} /> {t('nav.wallet', 'Wallet')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/settings/profile" {...props}>
							<Settings size={16} /> {t('nav.settings', 'Settings')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Separator class="my-1 h-px bg-[var(--color-border)]" />
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/logout" {...props}>
							<LogOut size={16} /> {t('nav.logout', 'Logout')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
			{:else}
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/login" {...props}>
							<LogIn size={16} /> {t('nav.login', 'Login')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/signup" {...props}>
							<User size={16} /> {t('nav.signup', 'Sign up')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
