<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organizations.md (my organizations)
	// Lists the orgs the current user is a member of, with a role badge and a
	// link to the per-org admin dashboard at /dashboard/organizations/{id}.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Building2, Plus, ShieldCheck } from 'lucide-svelte';
	import type { MyOrganizationRow } from './+page.server';

	let { data } = $props();
	const organizations = $derived(data.organizations);

	function roleKey(role: string): string {
		return `org_admin.role.${role}`;
	}
</script>

<section class="space-y-4">
	<header class="flex items-center justify-between">
		<div>
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.organizations')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('org_admin.list_subtitle')}</p>
		</div>
		<a
			href="/org/new"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			<Plus class="h-4 w-4" />
			{t('org_admin.new_org')}
		</a>
	</header>

	{#if organizations.length === 0}
		<EmptyState
			icon={Building2}
			title={t('org_admin.empty_title')}
			body={t('org_admin.empty_body')}
		/>
	{:else}
		<ul class="space-y-2">
			{#each organizations as o (o.id)}
				<li>
					<a
						href="/dashboard/organizations/{o.id}"
						class="group flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="flex min-w-0 items-center gap-3">
							<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
								<Building2 class="h-5 w-5" />
							</div>
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">
									{o.name}
									{#if o.verified}
										<ShieldCheck class="ml-1 inline h-3.5 w-3.5 text-[var(--color-primary)]" />
									{/if}
								</p>
								<p class="truncate text-xs text-[var(--color-muted)]">
									/{o.slug} · {o.member_count} {t('org_admin.members')}
								</p>
							</div>
						</div>
						<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-muted)]">
							{t(roleKey(o.member_role))}
						</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>
