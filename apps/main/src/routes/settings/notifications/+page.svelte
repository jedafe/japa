<script lang="ts">
	import { Bell, Mail, Smartphone, Users, MessageCircle, AtSign, Send, Loader2, Check } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import { api } from '$lib/api';
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';

	let { data, form } = $props();

	// svelte-ignore state_referenced_locally
	let prefs = $state(data.prefs);
	let saving = $state(false);

	const iconMap: Record<string, typeof Mail> = {
		email: Mail,
		push: Smartphone,
		follow: Users,
		comment: MessageCircle,
		mention: AtSign,
		dm: Send,
		group: Users,
		marketing: Mail
	};

	let notificationTypes = $state<{ key: string; label: string; description: string; icon: typeof Mail }[]>([]);
	let typesLoading = $state(true);

	let pushSupported = $state(false);
	let pushSubscribed = $state(false);
	let pushLoading = $state(false);
	let pushError = $state('');

	$effect(() => {
		if (browser && 'serviceWorker' in navigator && 'PushManager' in window) {
			pushSupported = true;
			navigator.serviceWorker.ready.then(async (reg) => {
				const sub = await reg.pushManager.getSubscription();
				pushSubscribed = !!sub;
			});
		}
	});

	async function togglePushSubscription() {
		pushLoading = true;
		pushError = '';
		try {
			const reg = await navigator.serviceWorker.ready;
			if (pushSubscribed) {
				const sub = await reg.pushManager.getSubscription();
				if (sub) await sub.unsubscribe();
				pushSubscribed = false;
			} else {
				const keyResult = await api.getPushVapidKey();
				const applicationServerKey = urlB64ToUint8Array(keyResult.public_key) as BufferSource;
				const sub = await reg.pushManager.subscribe({
					userVisibleOnly: true,
					applicationServerKey
				});
				await api.subscribePush(JSON.parse(JSON.stringify(sub)));
				pushSubscribed = true;
			}
		} catch (e) {
			pushError = e instanceof Error ? e.message : 'Failed to update push subscription';
		} finally {
			pushLoading = false;
		}
	}

	function urlB64ToUint8Array(base64String: string): Uint8Array {
		const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
		const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
		const raw = atob(base64);
		return Uint8Array.from([...raw].map((c) => c.charCodeAt(0)));
	}

	onMount(async () => {
		try {
			const types = await api.getNotificationTypes();
			if (Array.isArray(types)) {
				notificationTypes = types.map((t) => ({
					key: t.key,
					label: t.label,
					description: t.description,
					icon: iconMap[t.key] || Bell
				}));
			}
		} catch {
			notificationTypes = [];
		}
		typesLoading = false;
	});

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

	<!-- Web push subscription (separate from notification prefs — requires browser permission) -->
	{#if pushSupported}
		<section
			class="mt-6 rounded-2xl border p-5"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="flex items-start justify-between gap-4">
				<div>
					<h2 class="text-base font-semibold" style="color: var(--color-foreground)">
						Browser push notifications
					</h2>
					<p class="mt-1 text-sm" style="color: var(--color-muted)">
						{pushSubscribed
							? 'Your browser is subscribed and will receive real-time notifications.'
							: 'Subscribe to receive real-time alerts even when you\'re not on the site.'}
					</p>
					{#if pushError}
						<p class="mt-2 text-sm text-red-500">{pushError}</p>
					{/if}
				</div>
				<button
					onclick={togglePushSubscription}
					disabled={pushLoading}
					class="shrink-0 rounded-xl px-4 py-2 text-sm font-semibold transition-colors disabled:opacity-50"
					style={pushSubscribed
						? 'background: color-mix(in srgb, var(--color-destructive) 12%, transparent); color: var(--color-destructive)'
						: 'background: var(--color-primary); color: white'}
				>
					{#if pushLoading}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						{pushSubscribed ? 'Unsubscribe' : 'Enable push'}
					{/if}
				</button>
			</div>
		</section>
	{/if}
</main>
