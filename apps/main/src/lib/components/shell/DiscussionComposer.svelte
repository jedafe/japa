<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { Send, BarChart3, Plus, Trash2, ArrowLeft } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { searchSpaces } from '$lib/api/create';
	import { suggestSlug } from '$lib/components/create/slug';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import type { ForumTag, ForumThread, MeSpace } from '$lib/types';

	/**
	 * Inline discussion composer — rendered inside the Compose bottom sheet.
	 * Creates a forum thread via POST /forum, optionally with a poll via
	 * POST /forum/polls. Uses the TipTap PostEditor for rich text editing.
	 * Ref: ADR-058 (discussion posts compose inline) and
	 *      sections/08-forum-qa-solved.md
	 */

	interface Props {
		onClose: () => void;
		/** Whether the composer starts in question mode (set by the menu). */
		initialIsQuestion?: boolean;
	}

	let { onClose, initialIsQuestion = false }: Props = $props();

	// ---- form state ----
	let title = $state('');
	let bodyHtml = $state('');
	/** TipTap JSON document — base64-encoded for the forum API. */
	let bodyJson = $state<Record<string, unknown>>({ type: 'doc', content: [] });
	let spaces = $state<MeSpace[]>([]);
	let spaceId = $state<string>('');
	let isAnonymous = $state(false);
	let submitting = $state(false);
	let errorMsg = $state<string | null>(null);

	// Discussion vs Question toggle. Questions use the same editor but are
	// tagged "question" so they're filterable and eligible for the solved
	// flow. Seeded from the menu's Question button via initialIsQuestion.
	let isQuestion = $state(untrack(() => initialIsQuestion));

	// Tag chips (slugs). Attached after the thread is created via
	// POST /forum/{id}/tags (bulk, tag_ids). Slugs are resolved to IDs via
	// GET /forum/tags/{slug}; missing tags are created via POST /forum/tags.
	let tags = $state<string[]>([]);

	// Poll builder state.
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

	const canSubmit = $derived(title.trim().length > 0 && !!spaceId && !submitting);

	// Debounced space search — calls GET /me/spaces?q=… via the API client.
	async function spaceFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSpaces(q);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			label: s.name,
			sublabel: s.is_primary ? '★' : undefined
		}));
	}

	// Initial suggestions — show the user's spaces (primary first) when the
	// field is focused before any query is typed.
	async function spaceInitialFetcher(): Promise<AutocompleteItem[]> {
		return spaces
			.slice()
			.sort((a, b) => (a.is_primary ? -1 : b.is_primary ? 1 : 0))
			.slice(0, 5)
			.map((s) => ({
				id: s.id,
				label: s.name,
				sublabel: s.is_primary ? '★' : undefined
			}));
	}

	const selectedSpace = $derived(spaces.find((s) => s.id === spaceId) ?? null);

	// Resolve tag slugs to forum tag IDs, creating any that don't exist yet.
	// GET /forum/tags/{slug} → existing ID; POST /forum/tags → new tag.
	async function resolveTagIds(slugs: string[]): Promise<string[]> {
		const ids: string[] = [];
		for (const slug of slugs) {
			const getRes = await api.get<ForumTag>(`/forum/tags/${encodeURIComponent(slug)}`);
			if (!getRes.error && getRes.data) {
				ids.push(getRes.data.id);
				continue;
			}
			// Tag doesn't exist — create it (name derived from slug).
			const name = slug.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
			const createRes = await api.post<ForumTag>('/forum/tags', { name, slug });
			if (!createRes.error && createRes.data) ids.push(createRes.data.id);
		}
		return ids;
	}

	// Fetch the user's joined spaces on mount so the space selector is
	// populated. The sheet is client-side, so this uses the client API.
	onMount(async () => {
		const res = await api.get<MeSpace[]>('/me/spaces');
		if (!res.error && res.data) {
			spaces = res.data;
			spaceId = untrack(() => spaces.find((s) => s.is_primary)?.id ?? spaces[0]?.id ?? '');
		}
	});

	async function submit() {
		if (!canSubmit) return;
		submitting = true;
		errorMsg = null;

		const slug = suggestSlug(title);

		try {
			const poll =
				pollQuestion.trim() && pollOptions.filter((o) => o.trim()).length >= 2
					? {
							question: pollQuestion.trim(),
							is_multi_select: pollAllowMultiple,
							close_at: pollExpiresAt || null,
							options: pollOptions
								.filter((o) => o.trim())
								.map((label, i) => ({ label: label.trim(), position: i }))
						}
					: undefined;

			const res = await api.post<ForumThread>('/forum', {
				title: title.trim(),
				slug,
				body: btoa(JSON.stringify(bodyJson)),
				space_id: spaceId,
				is_anonymous: isAnonymous,
				poll
			});

			if (res.error) {
				errorMsg = res.error.message || t('forum.new.error_generic', 'Failed to create the discussion.');
				submitting = false;
				return;
			}

			const thread = res.data;
			if (!thread) {
				errorMsg = t('forum.new.error_generic', 'Failed to create the discussion.');
				submitting = false;
				return;
			}

			// Attach tags (non-fatal — the thread is already created).
			// Questions get a "question" tag so they're filterable.
			const effectiveTags = isQuestion && !tags.includes('question')
				? [...tags, 'question']
				: tags;
			if (effectiveTags.length > 0) {
				const tagIds = await resolveTagIds(effectiveTags);
				if (tagIds.length > 0) {
					await api.post(`/forum/${thread.id}/tags`, { tag_ids: tagIds });
				}
			}

			onClose();
			goto(`/forum/${thread.slug}`);
		} catch {
			errorMsg = t('forum.new.error_generic', 'Failed to create the discussion. Please try again.');
			submitting = false;
		}
	}
</script>

<div class="flex flex-col gap-4">
	<!-- Header -->
	<div class="flex items-center gap-3">
		<button
			type="button"
			onclick={() => onClose()}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('common.back', 'Back')}
		>
			<ArrowLeft size={18} />
		</button>
		<h2 class="text-base font-semibold text-[var(--color-text)]">
			{isQuestion ? t('forum.new.heading_question', 'New question') : t('forum.new.heading', 'New discussion')}
		</h2>
	</div>

	{#if errorMsg}
		<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
			{errorMsg}
		</p>
	{/if}

	{#if spaces.length === 0}
		<p class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-3 text-sm text-[var(--color-text-subtle)]">
			{t('forum.new.no_spaces', 'Join a space before starting a discussion.')}
		</p>
	{:else}
		<!-- Title -->
		<input
			id="dc-title"
			type="text"
			bind:value={title}
			placeholder={t('forum.new.placeholder_title', 'What do you want to discuss?')}
			maxlength="120"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		/>

		<!-- Space + Tags on the same row -->
		<div class="flex flex-wrap items-start gap-2">
			<div class="min-w-[160px] flex-1">
				<Autocomplete
					placeholder={t('create.placeholder.space_search', 'Search spaces…')}
					emptyMessage={t('create.placeholder.space_no_results', 'No spaces found')}
					fetcher={spaceFetcher}
					initialFetcher={spaceInitialFetcher}
					selectedId={spaceId || null}
					selectedLabel={selectedSpace?.name ?? null}
					selectedBadge={selectedSpace?.is_primary ? '★' : null}
					onSelect={(item) => (spaceId = item.id)}
					onClear={() => (spaceId = '')}
				/>
			</div>
			<div class="min-w-[180px] flex-1">
				<TagInput {tags} onTags={(next) => (tags = next)} />
			</div>
		</div>

		<!-- Body — TipTap rich text editor -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] focus-within:border-[var(--color-primary)]">
			<PostEditor
				html={bodyHtml}
				onedit={() => {}}
				onhtml={(v) => { bodyHtml = v; }}
				onjson={(v) => { bodyJson = v; }}
			/>
		</div>

		<!-- Anonymous toggle (left) + Type toggle (right) -->
		<div class="flex flex-wrap items-center justify-between gap-4">
			<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
				<input type="checkbox" bind:checked={isAnonymous} class="h-4 w-4 accent-[var(--color-primary)]" />
				{t('forum.new.anonymous', 'Post anonymously')}
			</label>
			<div class="inline-flex rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-0.5">
				<button
					type="button"
					onclick={() => (isQuestion = false)}
					class="rounded-[calc(var(--radius)-2px)] px-3 py-1 text-xs font-medium transition-colors {isQuestion ? 'text-[var(--color-muted)]' : 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm'}"
				>
					{t('create.discussion', 'Discussion')}
				</button>
				<button
					type="button"
					onclick={() => (isQuestion = true)}
					class="rounded-[calc(var(--radius)-2px)] px-3 py-1 text-xs font-medium transition-colors {isQuestion ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-muted)]'}"
				>
					{t('create.question', 'Question')}
				</button>
			</div>
		</div>

		<!-- Poll builder -->
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
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="mb-2 flex items-center justify-between">
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

				<label class="mb-2 block">
					<span class="mb-1 block text-xs text-[var(--color-muted)]">{t('forum.new.poll_question', 'Question')}</span>
					<input
						type="text"
						bind:value={pollQuestion}
						placeholder={t('forum.new.poll_question_placeholder', 'Ask a question…')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</label>

				<div class="mb-2 space-y-1.5">
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

				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
					<input type="checkbox" bind:checked={pollAllowMultiple} class="h-4 w-4 accent-[var(--color-primary)]" />
					{t('forum.new.poll_multiple', 'Allow multiple selections')}
				</label>
			</div>
		{/if}

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-3">
			<button
				type="button"
				onclick={() => onClose()}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				onclick={submit}
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				<Send size={16} /> {isQuestion ? t('forum.new.submit_question', 'Post question') : t('forum.new.submit', 'Post discussion')}
			</button>
		</div>
	{/if}
</div>
