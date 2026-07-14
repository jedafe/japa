<script lang="ts">
	import { enhance } from '$app/forms';
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import { t } from '$lib/stores/i18n.svelte';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { DropdownMenu } from 'bits-ui';

	let showDropdown = $state(false);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
	}
</script>

<header class="border-border-card bg-background-alt flex h-14 items-center justify-between border-b px-4">
	<div class="flex items-center gap-3">
		<a href="/" class="text-foreground text-sm font-semibold">{t('control.brand')}</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="bg-destructive rounded-5px px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<button
			type="button"
			class="text-muted-foreground hover:bg-dark-04 rounded-9px p-1.5 transition-colors"
			onclick={toggleTheme}
		>
			{$theme === 'dark' ? '☀️' : '🌙'}
		</button>

		{#if $user}
			<DropdownMenu.Root bind:open={showDropdown}>
				<DropdownMenu.Trigger
					class="hover:bg-dark-04 flex items-center gap-2 rounded-9px p-1.5 transition-colors outline-none"
				>
					<Avatar src={$user.avatar_url} alt={$user.display_name} size="sm" />
					<span class="text-foreground text-sm">{$user.display_name}</span>
				</DropdownMenu.Trigger>
				<DropdownMenu.Portal>
					<DropdownMenu.Content
						class="border-border-input bg-background shadow-popover z-50 min-w-[160px] rounded-xl border px-1 py-1.5"
						sideOffset={8}
						align="end"
					>
						<DropdownMenu.Item class="text-foreground data-highlighted:bg-accent flex cursor-pointer items-center rounded-9px px-3 py-2 text-sm outline-none">
							{$user.email}
						</DropdownMenu.Item>
						<DropdownMenu.Separator class="bg-border-card my-1 h-px" />
						<DropdownMenu.Item class="data-highlighted:bg-accent flex cursor-pointer items-center rounded-9px px-3 py-2 text-sm outline-none">
							<form method="POST" action="/logout" use:enhance class="w-full">
								<button type="submit" class="w-full text-left">{t('nav.sign_out')}</button>
							</form>
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Portal>
			</DropdownMenu.Root>
		{/if}
	</div>
</header>
