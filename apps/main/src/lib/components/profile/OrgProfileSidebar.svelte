<script lang="ts">
	import { t } from '$lib/i18n';
	import type { OrgMember } from '$lib/types';

	interface Props {
		members: OrgMember[];
	}
	let { members }: Props = $props();
</script>

{#if members.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('profile.members', '{n} members').replace('{n}', String(members.length))}</h2>
		<ul class="space-y-2">
			{#each members.slice(0, 8) as member (member.user.id)}
				<li>
					<a
						href={`/u/${member.user.username}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						{#if member.user.avatar_url}
							<img src={member.user.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
						{:else}
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
								{member.user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{member.user.display_name}</span>
							<span class="block text-xs text-[var(--color-muted)] capitalize">{member.role}</span>
						</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if members.length > 8}
			<p class="mt-2 text-xs text-[var(--color-muted)]">+{members.length - 8} {t('profile.more_members', 'more')}</p>
		{/if}
	</section>
{/if}
