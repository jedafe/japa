<script lang="ts">
	/**
	 * Quick thread composer — the collapsed "type a title and go" card on
	 * /thread and the space hub (§05 hub-redesign).
	 *
	 * The Ask | Discuss | Poll switch sets intent and swaps the input
	 * placeholder + action button:
	 * - Ask / Discuss: the input is the thread title — Enter (or the action
	 *   button) hands off to the compose sheet (DiscussionComposer) with the
	 *   title prefilled, question mode on for Ask.
	 * - Poll: the input is the poll question and the card expands an inline
	 *   option builder — the thread + embedded poll are created directly via
	 *   POST /thread (no sheet). The space target is the preset on hub pages
	 *   or a member-space picker on /thread.
	 */
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { BarChart3, HelpCircle, Loader2, MessageSquare, Plus, Trash2 } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { searchSpaces } from '#lib/api/create.js';
	import { encodeTiptapDoc } from '#lib/tiptap/serialize.js';
	import { suggestSlug } from '#lib/components/create/slug.js';
	import Autocomplete from '#lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import { compose } from '#lib/components/shell/compose.svelte.js';
	import { isAuthed, requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { ForumThread, MeSpace, Space } from '#lib/types/index.js';

	interface Props {
		/** Space-hub preset — locks the target space and swaps the copy. */
		space?: Space | null;
	}
	let { space = null }: Props = $props();

	type Mode = 'ask' | 'discuss' | 'poll';

	const avatar = $derived((page.data.user as { avatar_url?: string } | null)?.avatar_url ?? '');
	const initials = $derived(
		((page.data.user as { display_name?: string; username?: string } | null)?.display_name ??
			(page.data.user as { username?: string } | null)?.username ??
			'?'
		).charAt(0).toUpperCase()
	);

	let mode = $state<Mode>('discuss');
	/** The typed title — Ask/Discuss hand it to the sheet; Poll reuses it
	 *  as both the thread title and the poll question. */
	let title = $state('');

	// ---- inline poll builder (mode === 'poll') ----
	let pollOptions = $state<string[]>(['', '']);
	let pollAllowMultiple = $state(false);
	let pollExpiresAt = $state('');
	let pollOptionsEl = $state<HTMLDivElement | null>(null);

	// Space target — locked when a hub preset exists; otherwise the member's
	// spaces load lazily the first time Poll is selected (guests skip the
	// fetch — submit bounces them to /login before the space matters).
	let spaces = $state<MeSpace[]>([]);
	let spaceId = $state<string>(untrack(() => space?.id ?? ''));
	let spacesRequested = $state(false);
	let spacesLoaded = $state(false);
	let submitting = $state(false);
	let errorMsg = $state<string | null>(null);

	const preset = $derived(space ? { spaceId: space.id, spaceName: space.name } : null);

	const modes = $derived([
		{ value: 'ask' as Mode, label: t('forum.composer.ask', 'Ask'), icon: HelpCircle },
		{ value: 'discuss' as Mode, label: t('forum.composer.discuss', 'Discuss'), icon: MessageSquare },
		{ value: 'poll' as Mode, label: t('forum.composer.poll', 'Poll'), icon: BarChart3 }
	]);

	const placeholder = $derived.by(() => {
		if (mode === 'ask')
			return space
				? t('forum.composer.placeholder_ask_space', 'Ask {space} a question…', {
						space: space.name
					})
				: t('forum.composer.placeholder_ask', 'Ask the community a question…');
		if (mode === 'poll') return t('forum.composer.placeholder_poll', 'Ask a poll question…');
		return space
			? t('space.composer.placeholder', 'Share something with {space}…', { space: space.name })
			: t('forum.composer.placeholder', 'Share something with the community…');
	});

	const actionLabel = $derived(
		mode === 'ask'
			? t('forum.composer.ask', 'Ask')
			: mode === 'poll'
				? t('forum.create_poll', 'Create poll')
				: t('forum.composer.post', 'Post')
	);

	const filledOptions = $derived(pollOptions.filter((o) => o.trim().length > 0));
	const pollValid = $derived(title.trim().length > 0 && !!spaceId && filledOptions.length >= 2);
	// Guests keep an enabled action — submit bounces them to /login.
	const canAct = $derived(!submitting && (!isAuthed() || mode !== 'poll' || pollValid));

	const selectedSpace = $derived(spaces.find((s) => s.id === spaceId) ?? null);
	// Set only on a successful fetch — a failed fetch leaves the picker
	// usable (query search hits the API directly) without a false gate.
	const noSpaces = $derived(spacesLoaded && spaces.length === 0 && !preset);

	$effect(() => {
		if (mode !== 'poll' || preset || spacesRequested || !isAuthed()) return;
		spacesRequested = true;
		loadSpaces();
	});

	async function loadSpaces() {
		// member_only — the picker means "spaces I belong to"; without it
		// super admins get every platform space.
		const res = await api.get<MeSpace[]>('/me/spaces?member_only=1&page_size=50');
		if (!res.error && res.data) {
			spaces = res.data;
			spaceId = spaces.find((s) => s.is_primary)?.id ?? spaces[0]?.id ?? '';
			spacesLoaded = true;
		}
	}

	async function spaceFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSpaces(q);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			label: s.name,
			sublabel: s.is_primary ? '★' : undefined
		}));
	}

	// Initial suggestions — the member's spaces (primary first) when the
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

	function addPollOption() {
		pollOptions = [...pollOptions, ''];
	}
	function removePollOption(idx: number) {
		pollOptions = pollOptions.filter((_, i) => i !== idx);
	}
	function updatePollOption(idx: number, value: string) {
		pollOptions = pollOptions.map((o, i) => (i === idx ? value : o));
	}

	function focusOption(idx: number) {
		tick().then(() => {
			pollOptionsEl?.querySelectorAll<HTMLInputElement>('input')[idx]?.focus();
		});
	}

	/** Enter on the title: Ask/Discuss hand off to the sheet; Poll moves
	 *  into the option list (or submits a complete poll). */
	function onTitleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		if (mode === 'poll') {
			if (pollValid) {
				submitPoll();
			} else {
				const firstEmpty = pollOptions.findIndex((o) => !o.trim());
				focusOption(firstEmpty === -1 ? 0 : firstEmpty);
			}
		} else {
			openComposer();
		}
	}

	function onOptionKeydown(e: KeyboardEvent, idx: number) {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		if (pollValid) {
			submitPoll();
		} else if (idx === pollOptions.length - 1) {
			addPollOption();
			focusOption(idx + 1);
		} else {
			focusOption(idx + 1);
		}
	}

	/** Ask/Discuss — open the compose sheet with the typed title prefilled. */
	async function openComposer() {
		if (!(await requireAuthOrRedirect())) return;
		compose.openDiscussion(mode === 'ask', preset, title.trim());
	}

	/** Poll — create the thread + embedded poll directly (POST /thread). */
	async function submitPoll() {
		if (!(await requireAuthOrRedirect())) return;
		if (!pollValid || submitting) return;
		submitting = true;
		errorMsg = null;

		const res = await api.post<ForumThread>('/thread', {
			title: title.trim(),
			slug: suggestSlug(title),
			// The poll block renders its own question — the body carries an
			// empty paragraph so the contents row stays a valid TipTap doc.
			body: encodeTiptapDoc({ type: 'doc', content: [{ type: 'paragraph' }] }),
			space_id: spaceId,
			poll: {
				question: title.trim(),
				is_multi_select: pollAllowMultiple,
				close_at: pollExpiresAt ? new Date(pollExpiresAt).toISOString() : null,
				options: filledOptions.map((label, i) => ({ label: label.trim(), position: i }))
			}
		});

		const thread = res.data;
		if (res.error || !thread) {
			errorMsg = res.error?.message || t('forum.new.error_generic', 'Failed to create the discussion.');
			submitting = false;
			return;
		}
		goto(`/thread/${thread.slug}`);
	}

	function onAction() {
		if (mode === 'poll') submitPoll();
		else openComposer();
	}
</script>

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
	<div class="flex items-center gap-3">
		{#if avatar}
			<img src={remoteImageUrl(avatar)} alt="" class="h-9 w-9 shrink-0 rounded-full object-cover" />
		{:else}
			<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
				{initials}
			</span>
		{/if}
		<input
			type="text"
			bind:value={title}
			onkeydown={onTitleKeydown}
			{placeholder}
			maxlength="120"
			aria-label={placeholder}
			class="min-w-0 flex-1 rounded-full border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors focus:border-[var(--color-primary)] focus:outline-none"
		/>
	</div>

	{#if mode === 'poll'}
		<!-- Inline poll builder — question is the title input above. -->
		<div class="mt-3 border-t border-[var(--color-border)] pt-3">
			{#if errorMsg}
				<p class="mb-2 rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary-text)]">
					{errorMsg}
				</p>
			{/if}
			{#if noSpaces}
				<p class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]">
					{t('forum.new.no_spaces', 'Join a space before starting a discussion.')}
				</p>
			{:else}
				<div bind:this={pollOptionsEl} class="space-y-1.5">
					{#each pollOptions as opt, i (i)}
						<div class="flex items-center gap-2">
							<input
								type="text"
								value={opt}
								oninput={(e) => updatePollOption(i, e.currentTarget.value)}
								onkeydown={(e) => onOptionKeydown(e, i)}
								placeholder={t('forum.new.poll_option_placeholder', 'Option {n}').replace('{n}', String(i + 1))}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
							/>
							{#if pollOptions.length > 2}
								<button
									type="button"
									onclick={() => removePollOption(i)}
									class="shrink-0 rounded p-1.5 text-[var(--color-muted)] hover:bg-red-500/10 hover:text-red-500"
									aria-label={t('common.remove', 'Remove')}
								>
									<Trash2 size={14} />
								</button>
							{/if}
						</div>
					{/each}
				</div>
				<div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2">
					<button
						type="button"
						onclick={addPollOption}
						class="inline-flex items-center gap-1 text-xs text-[var(--color-primary-text)] hover:underline"
					>
						<Plus size={13} /> {t('forum.new.poll_add_option', 'Add option')}
					</button>
					<label class="flex cursor-pointer items-center gap-2 text-xs text-[var(--color-text-subtle)]">
						<input type="checkbox" bind:checked={pollAllowMultiple} class="h-3.5 w-3.5 accent-[var(--color-primary)]" />
						{t('forum.new.poll_multiple', 'Allow multiple selections')}
					</label>
					<input
						type="datetime-local"
						bind:value={pollExpiresAt}
						title={t('forum.new.poll_expires', 'Expires at (optional)')}
						aria-label={t('forum.new.poll_expires', 'Expires at (optional)')}
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				</div>
				{#if preset}
					<!-- Hub composer: the target space is fixed. -->
					<span class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)]">
						<span class="truncate">{preset.spaceName}</span>
					</span>
				{:else if isAuthed()}
					<div class="mt-2">
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
				{/if}
			{/if}
		</div>
	{/if}

	<div class="mt-2 flex items-center gap-1 border-t border-[var(--color-border)] pt-2">
		<!-- Intent switch — swaps placeholder + action per post kind. -->
		<div class="inline-flex rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-0.5" role="group" aria-label={t('forum.composer.mode', 'Post type')}>
			{#each modes as m (m.value)}
				{@const Icon = m.icon}
				<button
					type="button"
					onclick={() => (mode = m.value)}
					aria-pressed={mode === m.value}
					class="inline-flex items-center gap-1 rounded-[calc(var(--radius)-2px)] px-2.5 py-1 text-xs font-medium transition-colors {mode === m.value ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-muted)] hover:text-[var(--color-text-subtle)]'}"
				>
					<Icon size={13} class={mode === m.value ? 'text-[var(--color-primary-text)]' : ''} />
					{m.label}
				</button>
			{/each}
		</div>
		<button
			type="button"
			onclick={onAction}
			disabled={!canAct}
			class="ms-auto inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
		>
			{#if submitting}
				<Loader2 size={14} class="animate-spin" />
			{/if}
			{actionLabel}
		</button>
	</div>
</div>
