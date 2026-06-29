<script lang="ts">
	import { goto } from '$app/navigation';
	import { Mail, Lock, User, Loader2 } from 'lucide-svelte';

	let username = $state('');
	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			const res = await fetch('/api/v1/auth/register', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ username, email, password })
			});

			const body = await res.json();
			if (!res.ok || body.error) {
				error = body.error?.message || 'Registration failed. Please try again.';
				return;
			}

			goto('/login');
		} catch {
			error = 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register | JapaConnect</title>
	<meta name="description" content="Create your JapaConnect account." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Create an account</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Join the JapaConnect community
		</p>

		{#if error}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{error}
			</div>
		{/if}

		<form onsubmit={handleSubmit} class="mt-6 space-y-4">
			<div>
				<label for="username" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Username
				</label>
				<div class="relative">
					<User
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<input
						id="username"
						type="text"
						required
						minlength="3"
						maxlength="30"
						pattern="[a-zA-Z0-9_]+"
						bind:value={username}
						placeholder="johndoe"
						class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</div>
			</div>

			<div>
				<label for="email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Email
				</label>
				<div class="relative">
					<Mail
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<input
						id="email"
						type="email"
						required
						bind:value={email}
						placeholder="you@example.com"
						class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</div>
			</div>

			<div>
				<label for="password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Password
				</label>
				<div class="relative">
					<Lock
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<input
						id="password"
						type="password"
						required
						minlength="8"
						bind:value={password}
						placeholder="••••••••"
						class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</div>
			</div>

			<button
				type="submit"
				disabled={loading}
				class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if loading}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					Create account
				{/if}
			</button>
		</form>

		<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
			Already have an account?
			<a href="/login" class="font-medium hover:underline" style="color: var(--color-primary)">
				Sign in
			</a>
		</p>
	</div>
</main>
