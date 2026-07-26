<script lang="ts">
	import type { Snippet } from 'svelte';
	import { DropdownMenu } from 'bits-ui';
	import { Search, Sun, Moon, User, LogIn, LogOut } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
	import type { SessionUser, TagSuggestion, SpaceSuggestion } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		trendingTags?: TagSuggestion[];
		suggestedSpaces?: SpaceSuggestion[];
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
	}

	let { user, trendingTags = [], suggestedSpaces = [], sidebar }: Props = $props();
</script>

<aside class="flex h-full flex-col gap-4 overflow-y-auto px-4 py-3">
	<!-- Sticky top bar: search + avatar -->
	<div
		class="sticky top-0 z-10 -mx-4 flex items-center gap-2 bg-[var(--color-bg)]/90 px-4 py-2 backdrop-blur"
	>
		<form action="/search" method="GET" class="relative flex-1">
			<Search
				size={16}
				class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
			/>
			<input
				type="search"
				name="q"
				placeholder={t('sidebar.search_placeholder')}
				class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</form>

		<button
			type="button"
			onclick={() => theme.toggle()}
			class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
		>
			{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
		</button>

		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)] hover:ring-2 hover:ring-[var(--color-border)]"
				aria-label={t('nav.account')}
			>
				{#if user?.avatar_url}
					<img src={user.avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
				{:else if user}
					{user.displayName?.charAt(0)?.toUpperCase() ?? '?'}
				{:else}
					<User size={18} />
				{/if}
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<DropdownMenu.Content
					align="end"
					sideOffset={8}
					class="z-50 min-w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
				>
					{#if user}
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{#snippet child({ props })}
								<a href="/u/{user.username}" {...props}><User size={16} /> {user.displayName}</a>
							{/snippet}
						</DropdownMenu.Item>
						<DropdownMenu.Separator class="my-1 h-px bg-[var(--color-border)]" />
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{#snippet child({ props })}
								<a href="/logout" {...props}><LogOut size={16} /> {t('nav.logout')}</a>
							{/snippet}
						</DropdownMenu.Item>
					{:else}
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{#snippet child({ props })}
								<a href="/login" {...props}><LogIn size={16} /> {t('nav.login')}</a>
							{/snippet}
						</DropdownMenu.Item>
					{/if}
				</DropdownMenu.Content>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
	</div>

	<!-- Route-specific widgets -->
	{#if sidebar}
		{@render sidebar()}
	{/if}

	<!-- Global widget: trending tags -->
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('sidebar.trending_tags')}</h2>
			<ul class="flex flex-wrap gap-2">
				{#each trendingTags as tag (tag.id)}
					<li>
						<a
							href={`/tags/${tag.slug}`}
							class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
							>#{tag.name}</a
						>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- Global widget: suggested spaces -->
	{#if suggestedSpaces.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('sidebar.suggested_spaces')}</h2>
			<ul class="space-y-2">
				{#each suggestedSpaces as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								{#if space.member_count != null}
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
</aside>
