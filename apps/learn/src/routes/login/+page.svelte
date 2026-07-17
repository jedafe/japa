<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { enhance } from '$app/forms';
	import { PUBLIC_JC_SITE_URL } from '$env/static/public';
	import { GraduationCap, Mail, Lock } from 'lucide-svelte';
	import { Button, Input, Spinner } from '$lib/components/ui';

	let { form } = $props();

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
</script>

<svelte:head>
	<title>Sign In | JapaConnect Learn</title>
	<meta name="description" content="Sign in to JapaConnect Learn to track your progress and earn certificates." />
</svelte:head>

<main class="mx-auto flex min-h-[calc(100vh-12rem)] max-w-md items-center px-4 py-12 sm:px-6">
	<div class="w-full rounded-xl border p-6 sm:p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		<div class="mb-6 flex items-center gap-3">
			<div class="rounded-lg p-2" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)">
				<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-xl font-bold" style="color: var(--color-foreground)">{t('login.sign_in')}</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					Welcome to JapaConnect Learn
				</p>
			</div>
		</div>

		{#if form?.error}
			<div
				class="mb-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
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
				<label for="email" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">
					Email
				</label>
				<div class="relative">
					<Mail
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<Input
						id="email"
						name="email"
						type="email"
						required
						bind:value={email}
						placeholder="you@example.com"
						class="pl-10"
					/>
				</div>
			</div>

			<div>
				<label for="password" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">
					Password
				</label>
				<div class="relative">
					<Lock
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<Input
						id="password"
						name="password"
						type="password"
						required
						bind:value={password}
						placeholder="••••••••"
						class="pl-10"
					/>
				</div>
			</div>

			<Button
				type="submit"
				variant="primary"
				disabled={loading}
				loading={loading}
				class="w-full"
			>
				{#if loading}
					Signing in...
				{:else}
					Sign in
				{/if}
			</Button>
		</form>

		<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
			Don't have an account?
			<a href="{PUBLIC_JC_SITE_URL}/register" class="font-medium hover:underline" style="color: var(--color-primary)">
				Create one on JapaConnect
			</a>
		</p>
	</div>
</main>
