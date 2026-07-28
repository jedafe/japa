<script lang="ts">
	import { LogIn, BookOpen } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { Moon, Sun } from 'lucide-svelte';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
	}

	let { user }: Props = $props();

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
</script>

<div class="dropdown">
	<button
		type="button"
		class="toggle"
		onclick={() => (theme.toggle())}
		aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
		title={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
	>
		{#if theme.current === 'dark'}
			<Sun size={16} />
		{:else}
			<Moon size={16} />
		{/if}
	</button>

	{#if user}
		<a href="/docs" class="avatar" title={user.displayName}>
			{#if user.avatar_url}
				<img src={user.avatar_url} alt={user.displayName} class="avatar-img" />
			{:else}
				<span class="avatar-initials">{initials}</span>
			{/if}
		</a>
	{:else}
		<a href="/login" class="signin" title={t('audience.no_access_signin')}>
			<LogIn size={16} />
		</a>
	{/if}
</div>

<style>
	.dropdown {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}
	.toggle, .signin {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border-radius: var(--radius);
		color: var(--color-muted);
		background: transparent;
		border: 1px solid var(--color-border);
		cursor: pointer;
		text-decoration: none;
	}
	.toggle:hover, .signin:hover { color: var(--color-text); border-color: var(--color-border-input-hover); }
	.avatar {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border-radius: 9999px;
		overflow: hidden;
		border: 1px solid var(--color-border);
	}
	.avatar-img { height: 100%; width: 100%; object-fit: cover; }
	.avatar-initials {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-primary-contrast);
		background: var(--color-primary);
		height: 100%;
		width: 100%;
		display: grid;
		place-items: center;
	}
</style>
