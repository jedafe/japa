<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
	// Accepts an org invitation by POSTing the email token to
	// POST /organizations/invitations/{token}/accept. The backend verifies the
	// signed-in user's email matches the invited email, marks the invitation
	// accepted, adds the membership, and returns the org slug.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { Building2, Check, Loader2, Mail, ArrowRight } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let accepting = $state(false);
	let acceptedSlug = $state('');
	let errorMessage = $state('');

	const pageTitle = $derived(t('org_invite.title', 'Organization invitation — {site}'));

	async function accept() {
		if (accepting || acceptedSlug) return;
		accepting = true;
		errorMessage = '';
		try {
			const res = await api.post<{ message?: string; slug?: string }>(
				`/organizations/invitations/${encodeURIComponent(data.token)}/accept`
			);
			if (res.error) {
				errorMessage = res.error.message;
			} else {
				acceptedSlug = res.data?.slug ?? '';
			}
		} catch {
			errorMessage = t('org_invite.error', 'This invitation could not be accepted.');
		} finally {
			accepting = false;
		}
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-auto w-full max-w-3xl p-4">
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			{#if acceptedSlug}
				<div class="flex flex-col items-center gap-3 py-6 text-center">
					<span class="grid h-12 w-12 place-items-center rounded-full bg-[var(--color-success)]/10 text-[var(--color-success)]">
						<Check size={24} />
					</span>
					<h1 class="text-lg font-semibold text-[var(--color-text)]">
						{t('org_invite.success', "You're now a member of this organization.")}
					</h1>
					<a
						href="/org/{acceptedSlug}"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						{t('org_invite.view_org', 'View organization')}
						<ArrowRight size={15} />
					</a>
				</div>
			{:else}
				<div class="flex items-start gap-3">
					<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
						<Mail size={20} />
					</span>
					<div class="flex-1">
						<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
							<Building2 size={18} /> {t('org_invite.heading', 'Organization invitation')}
						</h1>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
							{t('org_invite.subtitle', 'Accept this invitation to join the organization as a member. The invitation is tied to the email address it was sent to.')}
						</p>
					</div>
				</div>

				{#if errorMessage}
					<p class="mt-4 rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">
						{errorMessage}
					</p>
				{/if}

				<div class="mt-5 flex justify-end">
					<button
						type="button"
						onclick={accept}
						disabled={accepting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if accepting}
							<Loader2 size={16} class="animate-spin" />
						{/if}
						{accepting ? t('common.loading', 'Loading…') : t('org_invite.accept', 'Accept invitation')}
					</button>
				</div>
			{/if}
		</section>
	</div>
</div>
