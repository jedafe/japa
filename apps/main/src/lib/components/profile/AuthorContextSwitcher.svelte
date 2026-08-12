<script lang="ts">
	// Ref: japaconnect-engineering/sections/33-professional-profiles.md (authorship context)
	// Allows the user to select whether they are posting as themselves or as
	// one of their professional profiles. The selected profile ID is emitted
	// via onSelected and should be sent as author_profile_id on content creation.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Briefcase, User, ChevronDown } from 'lucide-svelte';
	import type { ProfessionalProfile } from '$lib/types';

	interface Props {
		/** Currently selected author profile ID, or null for "self". */
		value?: string | null;
		/** Called when the user selects a different author context. */
		onSelected?: (profileId: string | null) => void;
	}

	let { value = null, onSelected }: Props = $props();

	let profiles = $state<ProfessionalProfile[]>([]);
	let loading = $state(true);
	let open = $state(false);

	$effect(() => {
		loadProfiles();
	});

	async function loadProfiles() {
		loading = true;
		try {
			const res = await api.get<{ items: ProfessionalProfile[] }>('/profiles?page_size=100');
			if (res.data) {
				profiles = (res.data as { items?: ProfessionalProfile[] }).items ?? [];
			}
		} catch {
			profiles = [];
		} finally {
			loading = false;
		}
	}

	const selectedProfile = $derived(profiles.find((p) => p.id === value) ?? null);

	function select(profileId: string | null) {
		open = false;
		onSelected?.(profileId);
	}
</script>

{#if !loading && profiles.length > 0}
	<div class="relative">
		<button
			type="button"
			onclick={() => (open = !open)}
			class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
		>
			{#if selectedProfile}
				<Briefcase class="h-4 w-4 text-[var(--color-primary)]" />
				<span>{selectedProfile.name}</span>
			{:else}
				<User class="h-4 w-4" />
				<span>{t('profiles.author_switcher.yourself')}</span>
			{/if}
			<ChevronDown class="h-3 w-3 text-[var(--color-muted)]" />
		</button>

		{#if open}
			<div class="absolute z-10 mt-1 w-56 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
				<button
					type="button"
					onclick={() => select(null)}
					class="flex w-full items-center gap-2 px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
				>
					<User class="h-4 w-4" />
					{t('profiles.author_switcher.yourself')}
				</button>
				{#each profiles as p (p.id)}
					<button
						type="button"
						onclick={() => select(p.id)}
						class="flex w-full items-center gap-2 px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
					>
						<Briefcase class="h-4 w-4 text-[var(--color-primary)]" />
						<span class="truncate">{p.name}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>
{/if}
