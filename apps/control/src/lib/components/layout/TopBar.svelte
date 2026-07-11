<script lang="ts">
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import Avatar from '$lib/components/ui/Avatar.svelte';

	let showDropdown = $state(false);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
	}

	function logout() {
		const form = document.createElement('form');
		form.method = 'POST';
		form.action = '/logout';
		document.body.appendChild(form);
		form.submit();
	}
</script>

<header class="flex h-14 items-center justify-between border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-900">
	<div class="flex items-center gap-3">
		<a href="/" class="text-sm font-semibold text-gray-900 dark:text-gray-100">JapaConnect Control</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<button
			type="button"
			class="rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
			onclick={toggleTheme}
		>
			{$theme === 'dark' ? '☀️' : '🌙'}
		</button>

		{#if $user}
			<div class="relative">
				<button
					type="button"
					class="flex items-center gap-2 rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-800"
					onclick={() => (showDropdown = !showDropdown)}
				>
					<Avatar src={$user.avatar_url} alt={$user.display_name} size="sm" />
					<span class="text-sm text-gray-700 dark:text-gray-300">{$user.display_name}</span>
				</button>

				{#if showDropdown}
					<div class="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800">
						<button
							type="button"
							class="w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
							onclick={logout}
						>
							Logout
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
