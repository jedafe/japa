<script lang="ts">
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
	// My Japa Stage. Server-preloaded via GET /me/japa-stage; updates go through
	// the browser API client to PATCH /me/japa-stage (auth). Shows the current
	// stage, a visual stepper of the 7-stage progression, and stage history.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import { ArrowLeft, Compass, Save } from 'lucide-svelte';
	import type { JapaStage, MyJapaStage } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const STAGES: JapaStage[] = [
		'considering',
		'planning',
		'preparing',
		'applied',
		'approved',
		'relocated',
		'settled'
	];

	const STAGE_COLORS: Record<JapaStage, string> = {
		considering: 'var(--color-muted)',
		planning: 'var(--stage-planning)',
		preparing: 'var(--stage-preparing)',
		applied: 'var(--stage-applied)',
		approved: 'var(--stage-approved)',
		relocated: 'var(--stage-relocated)',
		settled: 'var(--color-primary)'
	};

	function stageLabel(s: JapaStage): string {
		return t(`japa_stage.${s}`, s.charAt(0).toUpperCase() + s.slice(1));
	}

	function stageIndex(s: JapaStage): number {
		return STAGES.indexOf(s);
	}

	// Form state, seeded from the SSR-loaded stage.
	let stage = $state<JapaStage>(untrack(() => (data.myStage?.stage as JapaStage) ?? 'considering'));
	let destCountry = $state<string>(untrack(() => data.myStage?.destination_space_id ?? ''));
	let isPublic = $state<boolean>(untrack(() => data.myStage?.is_public ?? false));

	let loading = $state(false);
	let saved = $state(false);
	let errorKey = $state('');

	// Re-seed form state when the SSR data changes (e.g. after navigation).
	$effect(() => {
		const ms = data.myStage;
		if (ms) {
			stage = ms.stage as JapaStage;
			destCountry = ms.destination_space_id ?? '';
			isPublic = ms.is_public ?? false;
		}
	});

	const currentStage = $derived((data.myStage?.stage as JapaStage) ?? stage);
	const currentIndex = $derived(stageIndex(currentStage));

	const historyEntries = $derived(
		(data.myStage?.history as Array<{ stage: JapaStage; dest_space_id?: string; set_at: string }> | undefined) ?? []
	);

	// Resolve a dest_space_id UUID to the destination name using the
	// SSR-loaded countries list (avoids showing a raw UUID to the user).
	function destName(id?: string): string {
		if (!id) return '';
		const c = data.countries.find((c) => c.id === id);
		return c?.name ?? '';
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		saved = false;
		loading = true;
		try {
			const res = await api.patch<MyJapaStage>('/me/japa-stage', {
				stage,
				destination_space_id: destCountry || null,
				is_public: isPublic
			});
			if (res.data) {
				saved = true;
			} else {
				errorKey = 'japa_stage.errors.save_failed';
			}
		} catch {
			errorKey = 'japa_stage.errors.save_failed';
		} finally {
			loading = false;
		}
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<svelte:head>
	<title>{t('japa_stage.my_stage_title', 'My Japa Stage')} — JapaConnect</title>
	<meta name="description" content={t('japa_stage.my_stage_desc', 'Track and share your japa journey stage.')} />
</svelte:head>

<section class="mx-auto max-w-2xl space-y-6 px-4 py-6">
	<a
		href="/japa-stage"
		class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={14} /> {t('japa_stage.back_to_directory', 'Back to directory')}
	</a>

	<header class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
			<Compass class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">
				{t('japa_stage.my_stage_title', 'My Japa Stage')}
			</h1>
			<p class="text-sm text-[var(--color-muted)]">
				{t('japa_stage.my_stage_desc', 'Track and share your japa journey stage.')}
			</p>
		</div>
	</header>

	{#if !data.myStage}
		<EmptyState
			icon={Compass}
			title={t('japa_stage.not_set', 'No stage set yet')}
			body={t('japa_stage.not_set_body', 'Set your current japa stage below to appear in the directory.')}
		/>
	{/if}

	<!-- Visual stepper showing current stage position -->
	<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<p class="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--color-text-subtle)]">
			{t('japa_stage.progression', 'Stage progression')}
		</p>
		<ol class="flex flex-wrap items-center gap-1">
			{#each STAGES as s, i (s)}
				<li class="flex items-center gap-1">
					<div
						class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium"
						style={`border-color: ${i <= currentIndex ? STAGE_COLORS[s] : 'var(--color-border)'}; color: ${i <= currentIndex ? STAGE_COLORS[s] : 'var(--color-muted)'}; background-color: ${i === currentIndex ? `${STAGE_COLORS[s]}1a` : 'transparent'};`}
					>
						<span
							class="inline-block h-2 w-2 rounded-full"
							style={`background-color: ${i <= currentIndex ? STAGE_COLORS[s] : 'var(--color-border)'};`}
						></span>
						{stageLabel(s)}
					</div>
					{#if i < STAGES.length - 1}
						<span class="text-[var(--color-muted)]" aria-hidden="true">→</span>
					{/if}
				</li>
			{/each}
		</ol>
		{#if data.myStage?.updated_at}
			<p class="mt-3 text-xs text-[var(--color-text-subtle)]">
				{t('japa_stage.updated', 'Updated')} <RelativeTime iso={data.myStage.updated_at} />
			</p>
		{/if}
	</div>

	<!-- Update form -->
	<form onsubmit={submit} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
		<div class="flex flex-col gap-1.5">
			<label for="stage" class="text-sm font-medium text-[var(--color-text)]">
				{t('japa_stage.current_stage', 'Current stage')}
			</label>
			<select id="stage" bind:value={stage} class={inputClass}>
				{#each STAGES as s (s)}
					<option value={s}>{stageLabel(s)}</option>
				{/each}
			</select>
		</div>

		<div class="flex flex-col gap-1.5">
			<label for="dest" class="text-sm font-medium text-[var(--color-text)]">
				{t('japa_stage.dest_country', 'Destination country')}
			</label>
			<select id="dest" bind:value={destCountry} class={inputClass}>
				<option value="">{t('japa_stage.dest_placeholder', 'e.g. Canada')}</option>
				{#each data.countries as c (c.id)}
					<option value={c.id}>{c.name}</option>
				{/each}
			</select>
		</div>

		<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
			<input
				type="checkbox"
				bind:checked={isPublic}
				class="h-4 w-4 rounded border-[var(--color-border)]"
			/>
			{t('japa_stage.make_public', 'Show my stage in the public directory')}
		</label>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey)}
			</p>
		{/if}

		{#if saved}
			<p class="rounded-[var(--radius)] bg-green-500/10 px-3 py-2 text-sm text-green-600" role="status">
				{t('japa_stage.saved', 'Your japa stage has been updated.')}
			</p>
		{/if}

		<div class="flex justify-end pt-2">
			<button
				type="submit"
				disabled={loading}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
			>
				<Save class="h-4 w-4" />
				{loading ? '...' : t('japa_stage.save', 'Save')}
			</button>
		</div>
	</form>

	<!-- Stage history -->
	{#if historyEntries.length > 0}
		<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
				{t('japa_stage.history', 'Stage history')}
			</h2>
			<ol class="space-y-2">
				{#each historyEntries as entry, i (i)}
					<li class="flex items-center gap-2 text-sm">
						<span
							class="inline-block h-2 w-2 shrink-0 rounded-full"
							style={`background-color: ${STAGE_COLORS[entry.stage] ?? 'var(--color-muted)'};`}
						></span>
						<span class="font-medium text-[var(--color-text)]">{stageLabel(entry.stage)}</span>
						{#if entry.dest_space_id}
							<span class="text-[var(--color-text-subtle)]">→ {destName(entry.dest_space_id) || t('japa_stage.dest_set', 'Destination set')}</span>
						{/if}
						<span class="ml-auto text-xs text-[var(--color-text-subtle)]">
							<RelativeTime iso={entry.set_at} />
						</span>
					</li>
				{/each}
			</ol>
		</div>
	{/if}
</section>
