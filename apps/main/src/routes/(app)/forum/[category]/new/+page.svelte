<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { ArrowLeft, Send, BarChart3, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import type { PageData, ActionData } from './$types';
	import CommentEditor from '$lib/components/comments/CommentEditor.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// ?type=question — question chrome; the "question" tag is appended
	// server-side so the thread stays filterable for the solved flow.
	const isQuestion = $derived(data.isQuestion === true);

	let title = $state('');
	let body = $state<string>(''); // base64 TipTap JSON from CommentEditor
	let spaceId = $state<string>(untrack(() => data.preselected?.id ?? ''));
	let isAnonymous = $state(false);
	let tags = $state<string[]>([]);
	let submitting = $state(false);

	// Poll builder state (Phase 07.13).
	let showPoll = $state(false);
	let pollQuestion = $state('');
	let pollOptions = $state<string[]>(['', '']);
	let pollAllowMultiple = $state(false);
	let pollExpiresAt = $state('');

	function addPollOption() {
		pollOptions = [...pollOptions, ''];
	}
	function removePollOption(idx: number) {
		pollOptions = pollOptions.filter((_, i) => i !== idx);
		if (pollOptions.length < 2) pollOptions = ['', ''];
	}
	function updatePollOption(idx: number, value: string) {
		pollOptions = pollOptions.map((o, i) => (i === idx ? value : o));
	}

	const titleSlug = $derived(
		title
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '')
			.slice(0, 80)
	);

	const canSubmit = $derived(title.trim().length > 0 && !!spaceId && !!body && !submitting);

	const errorMsg = $derived(
		form?.error
			? form.error === 'missing_title'
				? t('forum.new.error_title', 'A title is required.')
				: form.error === 'missing_space'
					? t('forum.new.error_space', 'Select a space.')
					: t('forum.new.error_generic', 'Failed to create the discussion. Please try again.')
			: null
	);

	// Restore form state after a failed submission.
	$effect(() => {
		if (form?.title !== undefined) title = form.title;
		if (form?.body !== undefined) body = form.body;
		if (form?.spaceId !== undefined) spaceId = form.spaceId;
		if (form?.isAnonymous !== undefined) isAnonymous = form.isAnonymous;
		if (Array.isArray(form?.tags)) tags = form.tags;
	});

	// Autocomplete fetchers — filter the already-loaded spaces client-side.
	function fetchSpaces(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? data.spaces
			: data.spaces.filter((s) => s.name.toLowerCase().includes(query));
		return Promise.resolve(
			filtered.map((s) => ({
				id: s.id,
				label: s.name,
				sublabel: s.is_primary ? '★' : undefined
			}))
		);
	}
	function fetchSpacesInitial(): Promise<AutocompleteItem[]> {
		return fetchSpaces('');
	}
	const selectedSpaceLabel = $derived(
		spaceId ? data.spaces.find((s) => s.id === spaceId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{t('forum.new.title', 'New discussion — JapaConnect')}</title>
	<meta name="description" content={t('forum.new.description', 'Start a new discussion in the community.')} />
	<meta property="og:title" content={t('forum.new.title', 'New discussion')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-4 flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a href="/forum" class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]" aria-label={t('forum.new.back', 'Back to forum')}>
			<ArrowLeft size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{isQuestion ? t('forum.new.heading_question', 'New question') : t('forum.new.heading', 'New discussion')}
		</h1>
	</div>

	<form
		method="POST"
		use:enhance={() => {
			submitting = true;
			return async ({ update }) => {
				await update();
				submitting = false;
			};
		}}
		class="space-y-5 p-4"
	>
		{#if errorMsg}
			<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
				{errorMsg}
			</p>
		{/if}

		{#if isQuestion}
			<input type="hidden" name="type" value="question" />
		{/if}

		<!-- Space selector -->
		<div>
			<label for="space_id" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">{t('create.label.space', 'Space')}</label>
			<Autocomplete
				placeholder={t('create.placeholder.space', 'Select a space…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchSpaces}
				initialFetcher={fetchSpacesInitial}
				selectedId={spaceId || null}
				selectedLabel={selectedSpaceLabel}
				onSelect={(item) => (spaceId = item.id)}
				onClear={() => (spaceId = '')}
			/>
			<input type="hidden" name="space_id" value={spaceId} />
		</div>

		<!-- Tags (Section 08 — tags are first-class on discussions, max 5) -->
		<div>
			<label for="tags" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">{t('create.label.tags', 'Tags')}</label>
			<input type="hidden" name="tags" value={tags.join(',')} />
			<TagInput {tags} onTags={(next) => (tags = next)} maxTags={5} />
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('forum.new.tags_remaining', '{n} of 5 tags remaining').replace('{n}', String(Math.max(0, 5 - tags.length)))}
			</p>
		</div>

		<!-- Title -->
		<div>
			<label for="title" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">{t('forum.new.label_title', 'Title')}</label>
			<input
				id="title"
				name="title"
				type="text"
				bind:value={title}
				placeholder={t('forum.new.placeholder_title', 'What do you want to discuss?')}
				maxlength="120"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			{#if title}
				<p class="mt-1 text-xs text-[var(--color-muted)]">/forum/{titleSlug}</p>
			{/if}
		</div>

		<!-- Body -->
		<div>
			<label for="body" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">{t('forum.new.label_body', 'Body')}</label>
			<input type="hidden" name="body" value={body} />
			<CommentEditor placeholder={t('forum.new.placeholder_body', 'Write your discussion…')} oninput={(b) => (body = b)} />
		</div>

		<!-- Anonymous toggle -->
		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input type="checkbox" name="is_anonymous" bind:checked={isAnonymous} class="h-4 w-4 accent-[var(--color-primary)]" />
			{t('forum.new.anonymous', 'Post anonymously')}
		</label>

		<!-- Poll builder toggle (Phase 07.13) -->
		{#if !showPoll}
			<button
				type="button"
				onclick={() => (showPoll = true)}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<BarChart3 size={15} />
				{t('forum.new.add_poll', 'Add a poll')}
			</button>
		{:else}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="mb-3 flex items-center justify-between">
					<h3 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
						<BarChart3 size={15} /> {t('forum.new.poll', 'Poll')}
					</h3>
					<button
						type="button"
						onclick={() => { showPoll = false; pollQuestion = ''; pollOptions = ['', '']; pollAllowMultiple = false; pollExpiresAt = ''; }}
						class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
					>
						{t('common.remove', 'Remove')}
					</button>
				</div>

				<input type="hidden" name="poll_question" value={pollQuestion} />
				{#each pollOptions as opt, i (i)}
					<input type="hidden" name="poll_option[]" value={opt} />
				{/each}
				<input type="hidden" name="poll_allow_multiple" value={pollAllowMultiple ? 'on' : ''} />
				<input type="hidden" name="poll_expires_at" value={pollExpiresAt} />

				<label class="mb-3 block">
					<span class="mb-1 block text-xs text-[var(--color-muted)]">{t('forum.new.poll_question', 'Question')}</span>
					<input
						type="text"
						bind:value={pollQuestion}
						placeholder={t('forum.new.poll_question_placeholder', 'Ask a question…')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</label>

				<div class="mb-3 space-y-2">
					<span class="block text-xs text-[var(--color-muted)]">{t('forum.new.poll_options', 'Options')}</span>
					{#each pollOptions as opt, i (i)}
						<div class="flex items-center gap-2">
							<input
								type="text"
								value={opt}
								oninput={(e) => updatePollOption(i, e.currentTarget.value)}
								placeholder={t('forum.new.poll_option_placeholder', 'Option {n}').replace('{n}', String(i + 1))}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
							/>
							<button
								type="button"
								onclick={() => removePollOption(i)}
								class="shrink-0 rounded p-1.5 text-[var(--color-muted)] hover:bg-red-500/10 hover:text-red-500"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={14} />
							</button>
						</div>
					{/each}
					<button
						type="button"
						onclick={addPollOption}
						class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
					>
						<Plus size={13} /> {t('forum.new.poll_add_option', 'Add option')}
					</button>
				</div>

				<div class="flex flex-wrap gap-4">
					<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
						<input type="checkbox" bind:checked={pollAllowMultiple} class="h-4 w-4 accent-[var(--color-primary)]" />
						{t('forum.new.poll_multiple', 'Allow multiple selections')}
					</label>
				</div>

				<label class="mt-3 block">
					<span class="mb-1 block text-xs text-[var(--color-muted)]">{t('forum.new.poll_expires', 'Expires at (optional)')}</span>
					<input
						type="datetime-local"
						bind:value={pollExpiresAt}
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</label>
			</div>
		{/if}

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
			<a href="/forum" class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				<Send size={16} /> {isQuestion ? t('forum.new.submit_question', 'Post question') : t('forum.new.submit', 'Post discussion')}
			</button>
		</div>
	</form>
</div>
