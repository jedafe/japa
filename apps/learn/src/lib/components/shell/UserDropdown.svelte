<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import {
		User,
		LayoutDashboard,
		Bookmark,
		Trophy,
		Settings,
		LogOut,
		LogIn,
		GraduationCap
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { loginUrl, profileUrl, MAIN_ORIGIN } from '$lib/appOrigins';
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
		aria-label={t('nav.account')}
	>
		{#if user?.avatar_url}
			<img src={user.avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
		{:else if user}
			{user.displayName?.charAt(0)?.toUpperCase() ?? '?'}
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
						<a href={profileUrl(user.username)} {...props}>
							<User size={16} /> {t('nav.profile')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/my-learning" {...props}>
							<LayoutDashboard size={16} /> {t('learn.my_learning')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/certificates" {...props}>
							<Trophy size={16} /> {t('learn.certificates')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/bookmarks" {...props}>
							<Bookmark size={16} /> {t('learn.bookmarks')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href="/dashboard/instructor" {...props}>
							<GraduationCap size={16} /> {t('university.dashboard')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href={`${MAIN_ORIGIN}/settings`} {...props}>
							<Settings size={16} /> {t('nav.settings')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Separator class="my-1 h-px bg-[var(--color-border)]" />
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href={`${MAIN_ORIGIN}/logout`} {...props}>
							<LogOut size={16} /> {t('nav.logout')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
			{:else}
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href={loginUrl()} {...props}>
							<LogIn size={16} /> {t('nav.login')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
				<DropdownMenu.Item class={itemClass}>
					{#snippet child({ props })}
						<a href={`${MAIN_ORIGIN}/signup`} {...props}>
							<User size={16} /> {t('nav.signup')}
						</a>
					{/snippet}
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
