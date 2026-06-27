<script lang="ts">
	import { Bell, Check, CheckCheck } from 'lucide-svelte';
	import { DropdownMenu } from 'bits-ui';

	let {
		notifications = [],
		unreadCount = 0,
		onmarkread = () => {}
	}: {
		notifications: Array<{
			id: string;
			type: string;
			title: string;
			message: string;
			read: boolean;
			createdAt: string;
		}>;
		unreadCount: number;
		onmarkread: (id: string) => void;
	} = $props();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="relative rounded-md p-2 hover:bg-black/5 dark:hover:bg-white/10"
		aria-label="Notifications"
	>
		<Bell class="h-5 w-5" style="color: var(--color-muted)" />
		{#if unreadCount > 0}
			<span
				class="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-bold text-white"
				style="background-color: var(--color-primary)"
			>
				{unreadCount > 99 ? '99+' : unreadCount}
			</span>
		{/if}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content
		align="end"
		class="w-80 rounded-lg border p-1 shadow-lg"
		style="background-color: var(--color-background); border-color: var(--color-border)"
	>
		<div class="flex items-center justify-between px-3 py-2">
			<span class="text-sm font-semibold">Notifications</span>
			{#if unreadCount > 0}
				<button
					class="text-xs font-medium hover:underline"
					style="color: var(--color-primary)"
					onclick={() => onmarkread('all')}
				>
					Mark all read
				</button>
			{/if}
		</div>
		<DropdownMenu.Separator class="h-px" style="background-color: var(--color-border)" />

		{#if notifications.length === 0}
			<div class="px-3 py-8 text-center">
				<Bell class="mx-auto mb-2 h-8 w-8" style="color: var(--color-muted); opacity: 0.4" />
				<p class="text-sm" style="color: var(--color-muted)">No notifications yet</p>
			</div>
		{:else}
			<div class="max-h-80 overflow-y-auto">
				{#each notifications as notification}
					<DropdownMenu.Item
						class="flex cursor-pointer gap-3 rounded-md px-3 py-2 text-sm hover:bg-black/5 dark:hover:bg-white/10"
						style="opacity: {notification.read ? 0.6 : 1}"
					>
						<div class="flex-1 min-w-0">
							<p class="font-medium truncate">{notification.title}</p>
							<p class="truncate text-xs" style="color: var(--color-muted)">
								{notification.message}
							</p>
						</div>
						{#if !notification.read}
							<button
								class="mt-0.5 shrink-0 rounded-full p-0.5 hover:bg-black/10 dark:hover:bg-white/20"
								onclick={(e) => { e.stopPropagation(); onmarkread(notification.id); }}
								aria-label="Mark as read"
							>
								<Check class="h-3.5 w-3.5" style="color: var(--color-primary)" />
							</button>
						{/if}
					</DropdownMenu.Item>
				{/each}
			</div>
			<DropdownMenu.Separator class="h-px" style="background-color: var(--color-border)" />
			<a
				href="/notifications"
				class="block px-3 py-2 text-center text-sm font-medium hover:underline"
				style="color: var(--color-primary)"
			>
				View all notifications
			</a>
		{/if}
	</DropdownMenu.Content>
</DropdownMenu.Root>
