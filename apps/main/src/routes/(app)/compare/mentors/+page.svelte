<script lang="ts">
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
	import { t } from '$lib/i18n';
	import { GraduationCap, ArrowLeft } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { Mentor } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const mentors = $derived<Mentor[]>(data.mentors);

	const title = $derived(t('mentors.compare_title', 'Compare Mentors — JapaConnect'));
	const description = $derived(
		t('mentors.compare_description', 'Compare up to 4 mentors side-by-side.')
	);

	function capacityLabel(m: Mentor): string {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		return `${active}/${cap}`;
	}

	function availableSlots(m: Mentor): string {
		return String(m.available_slots ?? 0);
	}

	function areasList(m: Mentor): string {
		return m.mentor_areas?.join(', ') ?? '—';
	}

	// Comparison dimensions — each row compares one attribute across mentors.
	const dimensions = $derived([
		{
			label: t('mentors.compare_bio', 'About'),
			get: (m: Mentor) => m.mentor_bio ?? '—'
		},
		{
			label: t('mentors.compare_areas', 'Mentorship areas'),
			get: (m: Mentor) => areasList(m)
		},
		{
			label: t('mentors.compare_capacity', 'Mentees (active/capacity)'),
			get: (m: Mentor) => capacityLabel(m)
		},
		{
			label: t('mentors.compare_slots', 'Available slots'),
			get: (m: Mentor) => availableSlots(m)
		},
		{
			label: t('mentors.compare_since', 'Mentor since'),
			get: (m: Mentor) => (m.mentor_since ? new Date(m.mentor_since).getFullYear().toString() : '—')
		}
	]);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GraduationCap size={20} class="text-[var(--color-primary)]" />
			{t('mentors.compare_heading', 'Compare Mentors')}
		</h1>
		<a
			href="/mentors"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<ArrowLeft size={14} /> {t('mentors.browse', 'Browse mentors')}
		</a>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.compare_error', 'Could not load comparison')}
				body={t('mentors.compare_error_body', 'Please try again later.')}
				action={{ label: t('mentors.browse', 'Browse mentors'), href: '/mentors' }}
			/>
		{:else if mentors.length < 2}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.compare_empty', 'Select mentors to compare')}
				body={t(
					'mentors.compare_empty_body',
					'Go back to the mentor list and select 2-4 mentors to compare.'
				)}
				action={{ label: t('mentors.browse', 'Browse mentors'), href: '/mentors' }}
			/>
		{:else}
			<!-- Side-by-side comparison table -->
			<div class="overflow-x-auto">
				<table class="w-full border-collapse text-sm">
					<thead>
						<tr class="border-b border-[var(--color-border)]">
							<th class="w-40 p-3 text-left text-xs font-medium text-[var(--color-text-subtle)]">
								{t('mentors.compare_dimension', 'Dimension')}
							</th>
							{#each mentors as mentor (mentor.user_id)}
								<th class="p-3 text-left">
									<div class="flex items-center gap-2">
										{#if mentor.avatar_url}
											<img
												src={mentor.avatar_url}
												alt={mentor.username}
												class="h-8 w-8 rounded-full object-cover"
											/>
										{:else}
											<div class="grid h-8 w-8 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
												<GraduationCap size={16} />
											</div>
										{/if}
										<div class="min-w-0">
											<a
												href="/mentors/{mentor.username}"
												class="block truncate text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
											>
												{mentor.display_name ?? mentor.username}
											</a>
											<span class="text-xs text-[var(--color-muted)]">@{mentor.username}</span>
										</div>
									</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each dimensions as dim (dim.label)}
							<tr class="border-b border-[var(--color-border)]">
								<td class="p-3 text-xs font-medium text-[var(--color-text-subtle)]">
									{dim.label}
								</td>
								{#each mentors as mentor (mentor.user_id)}
									<td class="p-3 text-sm text-[var(--color-text)]">
										{dim.get(mentor)}
									</td>
								{/each}
							</tr>
						{/each}
						<!-- Action row -->
						<tr>
							<td class="p-3 text-xs font-medium text-[var(--color-text-subtle)]">
								{t('mentors.compare_action', 'Action')}
							</td>
							{#each mentors as mentor (mentor.user_id)}
								<td class="p-3">
									<a
										href="/mentors/{mentor.username}"
										class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
									>
										{t('mentors.request', 'Request Mentorship')}
									</a>
								</td>
							{/each}
						</tr>
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>
