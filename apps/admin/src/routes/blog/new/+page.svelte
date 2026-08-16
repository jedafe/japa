<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { blogAdminApi, contentAdminApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { ArrowLeft } from 'lucide-svelte';

	// Ref: jc-admin-develop — RBAC gate for blog creation (Section 24 — blog.create).
	// BUG-002: previously checked 'blog.post.create', a node that does not exist
	// in the permissions seed (migration 000109 seeds 'blog.create'), so the
	// create form was always blocked even for super_admin.
	const canCreate = $derived(hasPermission('blog.create'));

	let title = $state('');
	let slug = $state('');
	let bodyHtml = $state('');
	let status = $state('draft');
	let visibility = $state('public');
	let submitting = $state(false);
	let error = $state('');

	// BUG-007/BUG-008: fetch status & visibility options from the backend
	// instead of hardcoding them (Rule 16). Ref: §14 content status/visibility.
	let statusOptions = $state<{ value: string; label: string }[]>([]);
	let visibilityOptions = $state<{ value: string; label: string }[]>([]);

	$effect(() => {
		void (async () => {
			try {
				const [s, v] = await Promise.all([
					contentAdminApi.getStatusOptions(),
					contentAdminApi.getVisibilityOptions()
				]);
				if (s.ok) statusOptions = s.data ?? [];
				if (v.ok) visibilityOptions = v.data ?? [];
			} catch {
				// Best-effort — selectors fall back to the default-bound value.
			}
		})();
	});

	function generateSlug(val: string) {
		return val
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onTitleInput(e: Event) {
		const val = (e.currentTarget as HTMLInputElement).value;
		title = val;
		if (!slug || slug === generateSlug(title)) {
			slug = generateSlug(val);
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!title.trim()) {
			error = t('app.blog.error_title_required', 'Title is required');
			return;
		}
		submitting = true;
		error = '';
		try {
			const result = await blogAdminApi.createPost({
				title: title.trim(),
				slug: slug.trim() || undefined,
				body_html: bodyHtml || undefined,
				status,
				visibility
			});
			if (result.ok) {
				goto('/blog');
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_create', 'Failed to create blog post');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('app.blog.new_page_title', 'New Blog Post — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.blog.new_post', 'New Post')}
	description={t('app.blog.new_post_desc', 'Create a new blog post')}
	actions={[
		{ label: t('app.blog.back_to_list', 'Back to Blog'), icon: ArrowLeft, variant: 'ghost', onclick: () => goto('/blog') }
	]}
/>

{#if !canCreate}
	<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-danger); color: var(--color-danger)">
		<p class="text-sm font-medium">{t('common.access_denied', 'Access denied')}</p>
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('common.no_permission', 'You do not have permission to perform this action.')}</p>
	</div>
{:else}
<form onsubmit={handleSubmit} class="space-y-6">
	{#if error}
		<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<Card>
		<CardContent class="space-y-4 p-6">
			<div>
				<label for="post-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('app.blog.field_title', 'Title')}
				</label>
				<input
					id="post-title"
					type="text"
					value={title}
					oninput={onTitleInput}
					placeholder={t('app.blog.placeholder_title', 'Enter post title...')}
					class="w-full rounded-lg border p-3 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					required
				/>
			</div>

			<div>
				<label for="post-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('app.blog.field_slug', 'Slug')}
				</label>
				<input
					id="post-slug"
					type="text"
					bind:value={slug}
					placeholder={t('app.blog.placeholder_slug', 'post-url-slug')}
					class="w-full rounded-lg border p-3 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					{t('app.blog.slug_hint', 'Leave blank to auto-generate from title')}
				</p>
			</div>

			<div>
				<label for="post-body" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('app.blog.field_content', 'Content (HTML)')}
				</label>
				<textarea
					id="post-body"
					bind:value={bodyHtml}
					rows="12"
					placeholder={t('app.blog.placeholder_content', 'Write your post content here...')}
					class="w-full rounded-lg border p-3 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="post-status" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('app.blog.field_status', 'Status')}
					</label>
					<select
						id="post-status"
						bind:value={status}
						class="w-full rounded-lg border p-3 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						{#if statusOptions.length === 0}
							<option value="draft">{t('app.blog.status_draft', 'Draft')}</option>
						{:else}
							{#each statusOptions as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						{/if}
					</select>
				</div>
				<div>
					<label for="post-visibility" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('app.blog.field_visibility', 'Visibility')}
					</label>
					<select
						id="post-visibility"
						bind:value={visibility}
						class="w-full rounded-lg border p-3 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						{#if visibilityOptions.length === 0}
							<option value="public">{t('app.blog.visibility_public', 'Public')}</option>
						{:else}
							{#each visibilityOptions as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						{/if}
					</select>
				</div>
			</div>
		</CardContent>
	</Card>

	<div class="flex justify-end gap-3">
		<Button variant="ghost" onclick={() => goto('/blog')}>
			{t('app.blog.cancel', 'Cancel')}
		</Button>
		<Button type="submit" disabled={submitting}>
			{submitting ? t('app.blog.creating', 'Creating...') : t('app.blog.create_post', 'Create Post')}
		</Button>
	</div>
</form>
{/if}
