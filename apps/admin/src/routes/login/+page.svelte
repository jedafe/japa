<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, setToken } from '$lib/api';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function submit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
		const result = await api<{ access_token: string }>('/auth/login', {
			method: 'POST',
			body: { email, password }
		});
		if (result.ok) {
			const token = result.data.access_token;
			setToken(token);
			// Persist token in cookie for server-side access
			document.cookie = `admin_token=${token}; path=/; SameSite=Lax`;
				goto('/');
			} else {
				error = result.error || 'Invalid credentials';
			}
		} catch {
			error = 'Network error — is the backend running?';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h1 class="mb-1 text-center text-xl font-bold tracking-tight" style="color: var(--color-primary)">JapaAdmin</h1>
		<p class="mb-6 text-center text-sm" style="color: var(--color-muted)">Sign in to the admin panel</p>

		{#if error}
			<div class="mb-4 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{error}
			</div>
		{/if}

		<form onsubmit={submit} class="space-y-4">
			<div>
				<label for="email" class="mb-1 block text-sm font-medium">Email</label>
				<input
					id="email"
					type="email"
					required
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="admin@example.com"
					bind:value={email}
				/>
			</div>

			<div>
				<label for="password" class="mb-1 block text-sm font-medium">Password</label>
				<input
					id="password"
					type="password"
					required
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="••••••••"
					bind:value={password}
				/>
			</div>

			<button
				type="submit"
				class="w-full rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={loading || !email || !password}
			>
				{loading ? 'Signing in...' : 'Sign In'}
			</button>
		</form>
	</div>
</div>
