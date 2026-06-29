<script lang="ts">
	import { Bell, Mail, Smartphone, Users, MessageCircle, AtSign, Send, Loader2, Check } from 'lucide-svelte';
	import { enhance } from '$app/forms';

	let { data, form } = $props();

	let prefs = $state(data.prefs);
	let saving = $state(false);

	const notificationTypes = [
		{
			key: 'email',
			label: 'Email notifications',
			description: 'Receive email updates for important activity',
			icon: Mail
		},
		{
			key: 'push',
			label: 'Push notifications',
			description: 'Get browser push notifications',
			icon: Smartphone
		},
		{
			key: 'follow',
			label: 'Follow notifications',
			description: 'When someone follows you',
			icon: Users
		},
		{
			key: 'comment',
			label: 'Comment notifications',
			description: 'When someone comments on your posts',
			icon: MessageCircle
		},
		{
			key: 'mention',
			label: 'Mention notifications',
			description: 'When someone mentions you',
			icon: AtSign
		},
		{
			key: 'dm',
			label: 'Direct message notifications',
			description: 'When you receive a new message',
			icon: Send
		},
		{
			key: 'group',
			label: 'Group notifications',
			description: 'Updates from your spaces and groups',
			icon: Users
		},
		{
			key: 'marketing',
			label: 'Marketing emails',
			description: 'Product updates and community highlights',
			icon: Mail
		}
	];

	function toggleChannel(typeKey: string, channel: 'in_app' | 'email' | 'push') {
		prefs = {
			...prefs,
			[typeKey]: {
				...prefs[typeKey],
				[channel]: !prefs[typeKey][channel]
			}
		};
	}
</script>

<svelte:head>
	<title>Notification Settings | JapaConnect</title>
	<meta name="description" content="Manage your notification preferences." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
			<a href="/settings" class="hover:underline">Settings</a>
			<span>/</span>
			<span>Notifications</span>
		</div>
		<h1 class="mt-2 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			Notification Preferences
		</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Choose how you want to be notified about activity on JapaConnect.
		</p>
	</header>

	{#if form?.success}
		<div
			class="mb-6 flex items-center gap-2 rounded-lg border p-4"
			style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 10%, transparent)"
		>
			<Check class="h-5 w-5" style="color: var(--color-success)" />
			<span class="text-sm font-medium" style="color: var(--color-success)">
				Preferences saved successfully!
			</span>
		</div>
	{/if}

	{#if form?.error}
		<div
			class="mb-6 flex items-center gap-2 rounded-lg border p-4"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent)"
		>
			<span class="text-sm font-medium" style="color: var(--color-danger)">
				{form.error}
			</span>
		</div>
	{/if}

	<form
		method="POST"
		use:enhance={() => {
			saving = true;
			return async ({ update }) => {
				await update();
				saving = false;
			};
		}}
	>
		<div class="space-y-6">
			{#each notificationTypes as type (type.key)}
				<div
					class="rounded-xl border p-5 transition-colors hover:shadow-sm"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start gap-3 mb-4">
						<div
							class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
							style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)"
						>
							<type.icon class="h-4 w-4" style="color: var(--color-primary)" />
						</div>
						<div>
							<span class="text-sm font-medium" style="color: var(--color-foreground)">
								{type.label}
							</span>
							<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
								{type.description}
							</p>
						</div>
					</div>
					<div class="ml-11 flex gap-6">
						<label class="flex items-center gap-2 text-xs cursor-pointer" style="color: var(--color-muted)">
							<input
								type="checkbox"
								name="{type.key}_in_app"
								checked={prefs[type.key]?.in_app ?? false}
								onchange={() => toggleChannel(type.key, 'in_app')}
								class="h-3.5 w-3.5 rounded"
								style="accent-color: var(--color-primary)"
							/>
							In-app
						</label>
						<label class="flex items-center gap-2 text-xs cursor-pointer" style="color: var(--color-muted)">
							<input
								type="checkbox"
								name="{type.key}_email"
								checked={prefs[type.key]?.email ?? false}
								onchange={() => toggleChannel(type.key, 'email')}
								class="h-3.5 w-3.5 rounded"
								style="accent-color: var(--color-primary)"
							/>
							Email
						</label>
						<label class="flex items-center gap-2 text-xs cursor-pointer" style="color: var(--color-muted)">
							<input
								type="checkbox"
								name="{type.key}_push"
								checked={prefs[type.key]?.push ?? false}
								onchange={() => toggleChannel(type.key, 'push')}
								class="h-3.5 w-3.5 rounded"
								style="accent-color: var(--color-primary)"
							/>
							Push
						</label>
					</div>
				</div>
			{/each}
		</div>

		<div class="mt-8 flex items-center gap-4">
			<button
				type="submit"
				disabled={saving}
				class="flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if saving}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Bell class="h-4 w-4" />
				{/if}
				{saving ? 'Saving...' : 'Save preferences'}
			</button>
			<a
				href="/settings"
				class="rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:opacity-80"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>
				Cancel
			</a>
		</div>
	</form>
</main>
