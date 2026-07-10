<script lang="ts">
	import { enhance } from '$app/forms';

	let { form } = $props();

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
</script>

<svelte:head>
	<title>Login — JapaConnect Control</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
		<h1 class="mb-1 text-center text-xl font-bold tracking-tight text-gray-900 dark:text-gray-100">JapaConnect Control</h1>
		<p class="mb-6 text-center text-sm text-gray-500 dark:text-gray-400">Super Admin Console</p>

		{#if form?.error}
			<div class="mb-4 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-300">
				{form.error}
			</div>
		{/if}

		<form method="POST" use:enhance={() => {
			loading = true;
			return async ({ update }) => {
				await update();
				loading = false;
			};
		}} class="space-y-4">
			<div>
				<label for="email" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">Email</label>
				<input
					id="email"
					type="email"
					required
					class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
					placeholder="admin@example.com"
					bind:value={email}
				/>
			</div>

			<div>
				<label for="password" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">Password</label>
				<input
					id="password"
					type="password"
					required
					class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
					placeholder="••••••••"
					bind:value={password}
				/>
			</div>

			<button
				type="submit"
				class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
				disabled={loading || !email || !password}
			>
				{loading ? 'Signing in...' : 'Sign In'}
			</button>
		</form>
	</div>
</div>
