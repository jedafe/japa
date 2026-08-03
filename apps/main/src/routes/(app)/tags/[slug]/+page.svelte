<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { Dialog } from 'bits-ui';
	import { Hash, ArrowLeft, Clock, Eye, UserPlus, UserCheck, ExternalLink, BookOpen, Pencil, X, Loader2, Image as ImageIcon } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import type { MediaAsset } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tag = $derived(data.tag);
	const authed = $derived(!!page.data.user);
	const canManage = $derived(!!data.canManage);
	const fieldDefs = $derived(data.fieldDefs ?? []);
	const fieldValues = $derived(tag.field_values ?? {});

	// Filter to field defs that have a non-empty value in field_values.
	const visibleFields = $derived(
		fieldDefs.filter((f) => {
			const v = fieldValues[f.key];
			return v != null && String(v).trim() !== '';
		})
	);

	const title = $derived(t('tag.title', '#{name} — JapaConnect').replace('{name}', tag.name));
	const description = $derived(
		t('tag.description', 'Content tagged with #{name} on JapaConnect.').replace('{name}', tag.name)
	);
	const followerLabel = $derived(
		t('tag.followers', '{n} followers').replace('{n}', String(tag.follower_count))
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: `#${tag.name}`,
			description,
			url: page.url.href
		})
	);

	// Follow state for the tag. Hydrated server-side from
	// /users/{tag.id}/follow-status?entity_type=tag so the button reflects
	// the caller's actual follow state on SSR load. The backend exposes
	// POST/DELETE /tags/{slug}/follow (auth-gated).
	let following = $state(untrack(() => data.initiallyFollowing ?? false));
	let pending = $state(false);

	async function toggleFollow() {
		if (!authed || pending) return;
		const was = following;
		following = !was;
		pending = true;
		const res = was
			? await api.delete<{ following: boolean }>(`/tags/${tag.slug}/follow`)
			: await api.post<{ following: boolean }>(`/tags/${tag.slug}/follow`);
		if (res.error) {
			following = was;
		} else if (res.data && typeof res.data.following === 'boolean') {
			following = res.data.following;
		}
		pending = false;
	}

	// ---- Edit modal state ----
	// Only users with tags.manage permission see the edit button. The form
	// submits to PATCH /admin/tags/{id} — the same admin endpoint the admin
	// panel uses. The backend enforces the permission server-side.
	let editOpen = $state(false);
	let editName = $state('');
	let editDescription = $state('');
	let editRules = $state('');
	let editColor = $state('');
	let editLogoMediaId = $state('');
	let editLogoUrl = $state('');
	let editCoverMediaId = $state('');
	let editCoverUrl = $state('');
	let editFieldValues = $state<Record<string, string>>({});
	let editSaving = $state(false);
	let editError = $state('');
	let logoPickerOpen = $state(false);
	let coverPickerOpen = $state(false);

	function openEdit() {
		editName = tag.name;
		editDescription = tag.description ?? '';
		editRules = tag.rules ?? '';
		editColor = tag.color ?? '';
		editLogoMediaId = tag.logo_media_id ?? '';
		editLogoUrl = tag.logo_url ?? '';
		editCoverMediaId = tag.cover_media_id ?? '';
		editCoverUrl = tag.cover_url ?? '';
		const fv = tag.field_values ?? {};
		const stringFv: Record<string, string> = {};
		for (const [k, v] of Object.entries(fv)) {
			stringFv[k] = v == null ? '' : String(v);
		}
		editFieldValues = stringFv;
		editError = '';
		editOpen = true;
	}

	function handleLogoSelect(media: MediaAsset) {
		editLogoMediaId = media.id;
		editLogoUrl = media.url ?? '';
		logoPickerOpen = false;
	}

	function handleCoverSelect(media: MediaAsset) {
		editCoverMediaId = media.id;
		editCoverUrl = media.url ?? '';
		coverPickerOpen = false;
	}

	async function submitEdit() {
		if (!editName.trim() || editSaving) return;
		editSaving = true;
		editError = '';
		const body: Record<string, unknown> = {
			name: editName.trim(),
			description: editDescription.trim() || undefined,
			rules: editRules.trim() || undefined,
			color: editColor.trim() || undefined,
			logo_media_id: editLogoMediaId || undefined,
			cover_media_id: editCoverMediaId || undefined
		};
		if (fieldDefs.length > 0) {
			const fv: Record<string, string> = {};
			for (const def of fieldDefs) {
				const val = editFieldValues[def.key]?.trim();
				if (val) fv[def.key] = val;
			}
			body.field_values = fv;
		}
		const res = await api.patch(`/admin/tags/${tag.id}`, body);
		if (res.error) {
			editError = res.error.message;
		} else {
			editOpen = false;
			// Re-fetch the tag data so the page reflects the updated settings.
			await invalidate(`/api/v1/tags/${tag.slug}`);
		}
		editSaving = false;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<SetSidebar>
	<!-- Route-specific right sidebar: tag card with follow action -->
	<section
		class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
	>
		<div class="flex items-center gap-3">
			{#if tag.logo_url}
				<img
					src={tag.logo_url}
					alt={tag.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Hash size={24} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h2 class="truncate text-base font-semibold text-[var(--color-text)]">#{tag.name}</h2>
				<p class="text-xs text-[var(--color-muted)]">{followerLabel}</p>
			</div>
		</div>

		{#if tag.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{tag.description}</p>
		{/if}

		{#if authed}
			<button
				type="button"
				onclick={toggleFollow}
				disabled={pending}
				aria-busy={pending}
				class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 {following
					? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
					: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
			>
				{#if following}
					<UserCheck size={16} />
					{t('tag.following', 'Following')}
				{:else}
					<UserPlus size={16} />
					{t('tag.follow', 'Follow tag')}
				{/if}
			</button>
		{/if}
	</section>
</SetSidebar>

<!-- Center column: tag header + content grid -->
<div class="mx-auto w-full max-w-full">
	<!-- Cover image hero -->
	{#if tag.cover_url}
		<div class="relative h-32 w-full overflow-hidden border-b border-[var(--color-border)] sm:h-40">
			<img src={tag.cover_url} alt={tag.name} class="h-full w-full object-cover" />
		</div>
	{/if}

	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/discover?tab=tags"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} /> {t('tag.back', 'All tags')}
		</a>
		<div class="flex items-center gap-3">
			{#if tag.logo_url}
				<img
					src={tag.logo_url}
					alt={tag.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Hash size={24} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">#{tag.name}</h1>
				<p class="text-xs text-[var(--color-muted)]">{followerLabel}</p>
			</div>
			{#if canManage}
				<button
					type="button"
					onclick={openEdit}
					class="flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
				>
					<Pencil size={14} />
					{t('tag.edit', 'Edit Tag')}
				</button>
			{/if}
		</div>
		{#if tag.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{tag.description}</p>
		{/if}
		{#if tag.rules}
			<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					<BookOpen size={12} /> {t('tag.rules', 'Tag Rules')}
				</p>
				<p class="whitespace-pre-line text-sm text-[var(--color-text-subtle)]">{tag.rules}</p>
			</div>
		{/if}
		{#if visibleFields.length > 0}
			<div class="mt-4 space-y-3">
				{#each visibleFields as field (field.key)}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
						<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">{field.label}</p>
						{#if field.field_type === 'url'}
							<a
								href={String(fieldValues[field.key])}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
							>
								{String(fieldValues[field.key])}
								<ExternalLink size={12} />
							</a>
						{:else if field.field_type === 'rich_text'}
							<div class="prose prose-sm max-w-none text-sm text-[var(--color-text-subtle)]">
								{@html String(fieldValues[field.key])}
							</div>
						{:else}
							<p class="whitespace-pre-line text-sm text-[var(--color-text-subtle)]">{String(fieldValues[field.key])}</p>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('tag.load_error', 'Failed to load tagged content. Please try again later.')}
			</p>
		{:else if data.items.length === 0}
			<EmptyState icon={Hash} title={t('tag.empty', 'No content with this tag yet.')} />
		{:else}
			<!-- Magazine card grid (Ghost.org style, same as blog list) -->
			<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.items as post (post.id)}
					<a
						href={post.content_type === 'blog_post' ? `/blog/${post.slug}` : `/content/${post.slug}`}
						class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:border-[var(--color-primary)]/40 hover:shadow-lg"
					>
						{#if post.featured_image_url}
							<div class="aspect-[16/9] overflow-hidden">
								<img
									src={post.featured_image_url}
									alt={post.title}
									loading="lazy"
									class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
								/>
							</div>
						{:else}
							<div
								class="aspect-[16/9] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"
							></div>
						{/if}
						<div class="flex flex-1 flex-col p-5">
							<span
								class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-primary)]"
							>
								{t('tag.card_label', 'Tagged post')}
							</span>
							<h2
								class="text-xl font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
								style="font-family: var(--font-serif);"
							>
								{post.title}
							</h2>
							<p class="mt-2 line-clamp-3 flex-1 text-sm text-[var(--color-text-subtle)]">
								{post.body.slice(0, 160)}
							</p>
							<div class="mt-4 flex items-center gap-3 text-xs text-[var(--color-muted)]">
								{#if post.read_time_minutes > 0}
									<span class="flex items-center gap-1">
										<Clock size={12} /> {post.read_time_minutes} {t('tag.min_read', 'min read')}
									</span>
								{/if}
								<span class="flex items-center gap-1">
									<Eye size={12} /> {post.view_count}
								</span>
							</div>
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- Edit Tag Modal — visible only to users with tags.manage permission -->
<Dialog.Root bind:open={editOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-full max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
			<div class="mb-4 flex items-center justify-between">
				<Dialog.Title class="text-lg font-semibold text-[var(--color-text)]">
					{t('tag.edit_title', 'Edit Tag')}
				</Dialog.Title>
				<Dialog.Close class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
					<X size={18} />
				</Dialog.Close>
			</div>

			<div class="space-y-4">
				<!-- Name -->
				<div>
					<label for="edit-name" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.name', 'Name')}</label>
					<input id="edit-name" type="text" bind:value={editName} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" />
				</div>

				<!-- Description -->
				<div>
					<label for="edit-desc" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.description_label', 'Description')}</label>
					<textarea id="edit-desc" rows="2" bind:value={editDescription} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={t('tag.description_placeholder', 'Optional description...')}></textarea>
				</div>

				<!-- Rules -->
				<div>
					<label for="edit-rules" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.rules', 'Tag Rules')}</label>
					<textarea id="edit-rules" rows="3" bind:value={editRules} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={t('tag.rules_placeholder', 'Optional community rules/guidelines...')}></textarea>
				</div>

				<!-- Color -->
				<div>
					<label for="edit-color" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.color', 'Color')}</label>
					<div class="mt-1 flex items-center gap-2">
						<input id="edit-color" type="color" bind:value={editColor} class="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-surface)]" />
						<input type="text" bind:value={editColor} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm font-mono text-[var(--color-text)]" placeholder="#3b82f6" />
					</div>
				</div>

				<!-- Cover Image -->
				<div>
					<span class="block text-sm font-medium text-[var(--color-text)]">{t('tag.cover', 'Cover Image')}</span>
					<div class="mt-1 flex items-center gap-3">
						{#if editCoverUrl}
							<div class="relative">
								<img src={editCoverUrl} alt="Cover" class="h-12 w-20 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
								<button type="button" onclick={() => { editCoverMediaId = ''; editCoverUrl = ''; }} class="absolute -right-1 -top-1 rounded-full p-0.5" style="background: var(--color-danger); color: white" aria-label="Remove">
									<X size={12} />
								</button>
							</div>
						{:else}
							<div class="grid h-12 w-20 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
								<ImageIcon size={18} class="text-[var(--color-muted)]" />
							</div>
						{/if}
						<button type="button" onclick={() => (coverPickerOpen = true)} class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
							<ImageIcon size={14} />
							{t('tag.choose_cover', 'Choose Cover')}
						</button>
					</div>
				</div>

				<!-- Logo -->
				<div>
					<span class="block text-sm font-medium text-[var(--color-text)]">{t('tag.logo', 'Logo')}</span>
					<div class="mt-1 flex items-center gap-3">
						{#if editLogoUrl}
							<div class="relative">
								<img src={editLogoUrl} alt="Logo" class="h-12 w-12 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
								<button type="button" onclick={() => { editLogoMediaId = ''; editLogoUrl = ''; }} class="absolute -right-1 -top-1 rounded-full p-0.5" style="background: var(--color-danger); color: white" aria-label="Remove">
									<X size={12} />
								</button>
							</div>
						{:else}
							<div class="grid h-12 w-12 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
								<ImageIcon size={18} class="text-[var(--color-muted)]" />
							</div>
						{/if}
						<button type="button" onclick={() => (logoPickerOpen = true)} class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
							<ImageIcon size={14} />
							{t('tag.choose_logo', 'Choose Logo')}
						</button>
					</div>
				</div>

				<!-- Custom Fields -->
				{#if fieldDefs.length > 0}
					<div class="border-t border-[var(--color-border)] pt-3">
						<p class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('tag.custom_fields', 'Custom Fields')}</p>
						{#each fieldDefs as field (field.key)}
							<div class="mb-3">
								<label for="edit-field-{field.key}" class="block text-xs font-medium text-[var(--color-muted)]">
									{field.label}{#if field.is_required} <span style="color: var(--color-danger)">*</span>{/if}
								</label>
								{#if field.field_type === 'rich_text' || field.field_type === 'textarea'}
									<textarea id="edit-field-{field.key}" rows="3" bind:value={editFieldValues[field.key]} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={field.placeholder ?? ''}></textarea>
								{:else}
									<input id="edit-field-{field.key}" type={field.field_type === 'url' ? 'url' : 'text'} bind:value={editFieldValues[field.key]} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={field.placeholder ?? ''} />
								{/if}
							</div>
						{/each}
					</div>
				{/if}

				{#if editError}
					<p class="rounded-[var(--radius)] bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-400">{editError}</p>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button type="button" onclick={() => (editOpen = false)} class="rounded-[var(--radius)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
					{t('common.cancel', 'Cancel')}
				</button>
				<button type="button" onclick={submitEdit} disabled={editSaving} class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
					{#if editSaving}
						<Loader2 size={14} class="animate-spin" />
					{/if}
					{t('common.save', 'Save')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

{#if logoPickerOpen}
	<MediaLibraryModal onSelect={handleLogoSelect} onClose={() => (logoPickerOpen = false)} />
{/if}
{#if coverPickerOpen}
	<MediaLibraryModal onSelect={handleCoverSelect} onClose={() => (coverPickerOpen = false)} />
{/if}
