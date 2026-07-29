<script lang="ts">
	import { t } from '$lib/i18n';
	import { Award } from 'lucide-svelte';
	import FollowButton from './FollowButton.svelte';
	import type { PublicUser, ProfileCertificate, ProfileSpace } from '$lib/types';

	interface Props {
		user: PublicUser;
		isOwnProfile: boolean;
		certificates: ProfileCertificate[];
		spaces: ProfileSpace[];
	}
	let { user, isOwnProfile, certificates, spaces }: Props = $props();
</script>

<!-- Route-specific right sidebar for /u/[username] — rendered above global widgets. -->
{#if !isOwnProfile}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<FollowButton userId={user.id} />
	</section>
{/if}

{#if certificates.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('profile.certificates', 'Certificates')}</h2>
		<ul class="space-y-2">
			{#each certificates as cert (cert.id)}
				<li>
					<a
						href={`/certificates/${cert.id}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
							<Award size={16} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{cert.title ?? cert.id}</span>
							{#if cert.issued_at}
								<span class="block text-xs text-[var(--color-muted)]">{cert.issued_at}</span>
							{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if spaces.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('profile.spaces', 'Spaces')}</h2>
		<ul class="space-y-2">
			{#each spaces as space (space.id)}
				<li>
					<a
						href={`/spaces/${space.slug}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
							{#if space.role}
								<span class="block text-xs text-[var(--color-muted)]">{space.role}</span>
							{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}
