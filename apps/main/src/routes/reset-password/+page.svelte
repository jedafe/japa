<script lang="ts">
	import { page } from '$app/state';
	import { Lock, Eye, EyeOff, Loader2, Check } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Button, Input } from '$lib/components/ui';

	let password = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let loading = $state(false);
	let success = $state(false);
	let showPassword = $state(false);
	let showConfirm = $state(false);

	let hasMinLength = $derived(password.length >= 8);
	let hasNumber = $derived(/\d/.test(password));
	let hasSpecial = $derived(/[!@#$%^&*()_+\-=[\]{};':"\\|,.<>/?`~]/.test(password));
	let strengthCount = $derived(Number(hasMinLength) + Number(hasNumber) + Number(hasSpecial));

	let strengthLabel = $derived(
		strengthCount === 0 ? '' : strengthCount === 1 ? 'Weak' : strengthCount === 2 ? 'Fair' : 'Strong'
	);

	let strengthColor = $derived(
		strengthCount <= 1
			? 'var(--color-danger)'
			: strengthCount === 2
				? 'var(--color-warning, #eab308)'
				: 'var(--color-success, #22c55e)'
	);

	function getToken(): string {
		return page.url.searchParams.get('token') ?? '';
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';

		const token = getToken();
		if (!token) {
			error = 'Invalid or missing reset token.';
			return;
		}

		if (password !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		if (!hasMinLength || !hasNumber || !hasSpecial) {
			error = 'Password does not meet the strength requirements.';
			return;
		}

		loading = true;

		try {
			await api.resetPassword(token, password);
			success = true;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Reset Password | JapaConnect</title>
	<meta name="description" content="Set a new password for your JapaConnect account." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		{#if success}
			<div class="text-center">
				<div
					class="mx-auto flex h-12 w-12 items-center justify-center rounded-full"
					style="background-color: color-mix(in srgb, var(--color-success, #22c55e) 15%, transparent)"
				>
					<Check class="h-6 w-6" style="color: var(--color-success, #22c55e)" />
				</div>
				<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.password_reset')}</h1>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{t('auth.password_reset_msg')}
				</p>
				<a
					href="/login"
					class="mt-6 inline-flex items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
				>
					{t('auth.sign_in')}
				</a>
			</div>
		{:else}
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.reset_password')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Enter your new password below.
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
					<label for="password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('auth.new_password')}
					</label>
					<div class="relative">
						<Lock
							class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
							style="color: var(--color-muted)"
						/>
						<Input
							id="password"
							type={showPassword ? 'text' : 'password'}
							required
							minlength="8"
							bind:value={password}
							placeholder="••••••••"
							class="pl-10 pr-10"
						/>
						<button
							type="button"
							onclick={() => (showPassword = !showPassword)}
							class="absolute right-3 top-1/2 -translate-y-1/2"
							style="color: var(--color-muted)"
							aria-label={showPassword ? t('auth.hide_password') : t('auth.show_password')}
						>
							{#if showPassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>

					<!-- Password strength indicator -->
					{#if password.length > 0}
						<div class="mt-2 space-y-2">
							<div class="flex gap-1">
								{#each [1, 2, 3] as i}
									<div
										class="h-1 flex-1 rounded-full transition-colors"
										style="background-color: {i <= strengthCount ? strengthColor : 'var(--color-border)'}"
									></div>
								{/each}
							</div>
							<div class="flex flex-wrap gap-x-4 gap-y-1">
								<span class="flex items-center gap-1 text-xs" style="color: {hasMinLength ? strengthColor : 'var(--color-muted)'}">
									<Check class="h-3 w-3" style="opacity: {hasMinLength ? 1 : 0.3}" />
									{t('auth.8_plus_chars')}
								</span>
								<span class="flex items-center gap-1 text-xs" style="color: {hasNumber ? strengthColor : 'var(--color-muted)'}">
									<Check class="h-3 w-3" style="opacity: {hasNumber ? 1 : 0.3}" />
									{t('auth.a_number')}
								</span>
								<span class="flex items-center gap-1 text-xs" style="color: {hasSpecial ? strengthColor : 'var(--color-muted)'}">
									<Check class="h-3 w-3" style="opacity: {hasSpecial ? 1 : 0.3}" />
									{t('auth.a_special')}
								</span>
							</div>
						</div>
					{/if}
				</div>

				<div>
					<label for="confirm_password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('auth.confirm_password')}
					</label>
					<div class="relative">
						<Lock
							class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
							style="color: var(--color-muted)"
						/>
						<Input
							id="confirm_password"
							type={showConfirm ? 'text' : 'password'}
							required
							bind:value={confirmPassword}
							placeholder="••••••••"
							class="pl-10 pr-10"
						/>
						<button
							type="button"
							onclick={() => (showConfirm = !showConfirm)}
							class="absolute right-3 top-1/2 -translate-y-1/2"
							style="color: var(--color-muted)"
							aria-label={showConfirm ? t('auth.hide_password') : t('auth.show_password')}
						>
							{#if showConfirm}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>
				</div>

				<Button
					type="submit"
					disabled={loading}
					loading={loading}
					class="w-full"
				>
					{#if !loading}
						{t('auth.reset_password')}
					{/if}
				</Button>
			</form>

			<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
				{t('auth.remember_password')}
				<a href="/login" class="font-medium hover:underline" style="color: var(--color-primary)">
					{t('auth.sign_in')}
				</a>
			</p>
		{/if}
	</div>
</main>
