<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { setAccessToken } from '$lib/api/token';

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let error = $state('');

	async function handleLogin() {
		if (!email || !password || loading) return;
		
		loading = true;
		error = '';
		
		try {
			// Call login endpoint directly with fetch to get cookies set
			const res = await fetch('/api/v1/auth/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ email, password })
			});

			// Defensively parse: a failed/empty response (network error, proxy
			// down, or non-JSON error body) must not throw "Unexpected end of
			// JSON input". Read as text first, then attempt JSON decode.
			const text = await res.text();
			let body: { data?: { access_token?: string }; error?: { message?: string } } = {};
			if (text) {
				try {
					body = JSON.parse(text);
				} catch {
					body = {};
				}
			}

			if (res.ok && body.data?.access_token) {
				setAccessToken(body.data.access_token);
				// Hard reload so the root layout re-mounts and onMount re-runs
				// with the fresh access token (fetches /me + /me/permissions).
				// Client-side goto() would NOT re-run onMount since the layout
				// is already mounted from the initial page load.
				window.location.href = '/';
			} else if (res.ok) {
				// Authenticated but no access token (e.g. TOTP challenge) — surface it.
				error = body.error?.message || 'Login succeeded but no session was issued';
			} else {
				const status = res.status ? ` (${res.status})` : '';
				error = body.error?.message || `Login failed${status}`;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<h1 class="mb-1 text-center text-xl font-bold tracking-tight" style="color: var(--color-foreground)">{t('auth.login.japaconnect_admin')}</h1>
		<p class="mb-6 text-center text-sm" style="color: var(--color-muted)">{t('auth.login.content_moderation_management')}</p>

		{#if error}
			<div class="mb-4 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
				{error}
			</div>
		{/if}

		<form onsubmit={(e) => { e.preventDefault(); handleLogin(); }} class="space-y-4">
			<div>
				<label for="email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('auth.login.email')}</label>
				<input
					id="email"
					type="email"
					required
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder="admin@example.com"
					bind:value={email}
				/>
			</div>

			<div>
				<label for="password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('auth.login.password')}</label>
				<input
					id="password"
					type="password"
					required
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder="••••••••"
					bind:value={password}
				/>
			</div>

			<button
				type="submit"
				class="w-full rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background: var(--color-primary)"
				disabled={loading || !email || !password}
			>
				{loading ? 'Signing in...' : 'Sign In'}
			</button>
		</form>
	</div>
</div>
