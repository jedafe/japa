<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { setAccessToken } from '$lib/api/token';
	import { restoreSession } from '$lib/stores/session.svelte';
	import { goto } from '$app/navigation';
	import { getPermissions } from '$lib/stores/permissions.svelte';
	import { setChallenge } from '$lib/api/challenge';

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
			let body: { data?: { access_token?: string; totp_required?: boolean; challenge_token?: string; user?: { email?: string } }; error?: { message?: string } } = {};
			if (text) {
				try {
					body = JSON.parse(text);
				} catch {
					body = {};
				}
			}

			if (res.ok && body.data?.access_token) {
				setAccessToken(body.data.access_token);
				// Restore the session in-place (fetches /me + /me/permissions
				// and populates the auth stores) then navigate to the admin
				// root. invalidateAll triggers the layout's $effect/reactive
				// dependencies so the permission-gated UI re-evaluates.
				await restoreSession();
				// Touch getPermissions so the layout's currentPermissions
				// reactivity picks up the new state.
				void getPermissions();
				await goto('/', { invalidateAll: true });
			} else if (res.ok && body.data?.totp_required) {
				// 2FA challenge required (§02/§22). Stash the challenge token
				// + email and redirect to the verify-totp page so the admin
				// can complete the second factor. Without this, an admin with
				// 2FA enabled (mandatory per §22) would be locked out with a
				// generic "no session was issued" error.
				setChallenge(body.data.challenge_token ?? null, body.data.user?.email ?? email);
				await goto('/verify-totp');
			} else if (res.ok) {
				// Authenticated but no access token and no TOTP challenge — surface it.
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
				{loading ? t('login.signing_in', 'Signing in...') : t('login.submit', 'Sign In')}
			</button>
		</form>
	</div>
</div>
