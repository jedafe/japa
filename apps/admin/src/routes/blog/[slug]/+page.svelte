<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { api, blogAdminApi, contentAdminApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { ArrowLeft, Save, Trash2, CheckCircle, XCircle, RefreshCw } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	// Form fields
	let postId = $state('');
	let title = $state('');
	let slug = $state('');
	let bodyHtml = $state('');
	let status = $state('draft');
	let visibility = $state('public');

	// BUG-007/BUG-008: fetch status & visibility options from the backend
	// instead of hardcoding them (Rule 16). Ref: §14 content status/visibility.
	let statusOptions = $state<{ value: string; label: string }[]>([]);
	let visibilityOptions = $state<{ value: string; label: string }[]>([]);

	// Original status for action buttons
	let originalStatus = $state('');

	// Delete confirmation dialog state (replaces window.confirm)
	let showDeleteConfirm = $state(false);

	const canManageBlog = $derived(hasPermission('blog.review.manage'));
	const slugParam = $derived(page.params.slug);

	function generateSlug(val: string) {
		return val
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	// Ref: jc-admin-develop — use $effect so param changes re-fetch (SPA architecture)
	$effect(() => {
		void slugParam;
		loadPost();
	});

	// BUG-007/BUG-008: fetch status & visibility options once on mount (Rule 16).
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

	async function loadPost() {
		loading = true;
		error = '';
		try {
			// Try to get by slug first, then by ID
			let result = await api<any>(`/blog/${slugParam}`);
			if (!result.ok) {
				// Try admin endpoint
				result = await api<any>(`/admin/blog`);
				if (result.ok && Array.isArray(result.data)) {
					const found = result.data.find((p: any) => p.slug === slugParam || p.id === slugParam);
					if (found) {
						result = { ok: true, data: found };
					}
				}
			}
			if (result.ok && result.data) {
				const post = result.data;
				postId = post.id;
				title = post.title || '';
				slug = post.slug || '';
				status = post.status || 'draft';
				originalStatus = status;
				visibility = post.visibility || 'public';
				// Try to extract HTML from body_html or body
				if (post.body_html) {
					bodyHtml = post.body_html;
				} else if (post.body) {
					try {
						const parsed = JSON.parse(post.body);
						bodyHtml = JSON.stringify(parsed);
					} catch {
						bodyHtml = post.body;
					}
				}
			} else {
				error = t('app.blog.error_not_found', 'Blog post not found');
			}
		} catch (e) {
			error = t('app.blog.error_load', 'Failed to load blog post');
		} finally {
			loading = false;
		}
	}

	async function handleSave() {
		if (!title.trim()) {
			error = t('app.blog.error_title_required', 'Title is required');
			return;
		}
		saving = true;
		error = '';
		success = '';
		try {
			const result = await blogAdminApi.updatePost(postId, {
				title: title.trim(),
				slug: slug.trim() || undefined,
				body_html: bodyHtml || undefined,
				status,
				visibility
			});
			if (result.ok) {
				success = t('app.blog.success_saved', 'Post saved successfully');
				originalStatus = status;
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_save', 'Failed to save post');
		} finally {
			saving = false;
		}
	}

	async function handlePublish() {
		saving = true;
		error = '';
		try {
			const result = await blogAdminApi.publishPost(postId);
			if (result.ok) {
				status = 'published';
				originalStatus = 'published';
				success = t('app.blog.success_published', 'Post published successfully');
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_publish', 'Failed to publish post');
		} finally {
			saving = false;
		}
	}

	async function handleUnpublish() {
		saving = true;
		error = '';
		try {
			const result = await blogAdminApi.unpublishPost(postId);
			if (result.ok) {
				status = 'draft';
				originalStatus = 'draft';
				success = t('app.blog.success_unpublished', 'Post unpublished successfully');
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_unpublish', 'Failed to unpublish post');
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		saving = true;
		error = '';
		try {
			const result = await blogAdminApi.deletePost(postId);
			if (result.ok) {
				showDeleteConfirm = false;
				goto('/blog');
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_delete', 'Failed to delete post');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{t('app.blog.edit_page_title', 'Edit Blog Post — {site} Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.blog.edit_post', 'Edit Post')}
	description={t('app.blog.edit_post_desc', 'Edit blog post content and settings')}
	actions={[
		{ label: t('app.blog.back_to_list', 'Back to Blog'), icon: ArrowLeft, variant: 'ghost', onclick: () => goto('/blog') }
	]}
/>

{#if loading}
	<Card>
		<CardContent class="p-6">
			<TableSkeleton rows={5} columns={1} />
		</CardContent>
	</Card>
{:else if error && !postId}
	<Card>
		<CardContent class="p-6">
			<div class="text-center" style="color: var(--color-muted)">
				<p>{error}</p>
				<Button class="mt-4" onclick={() => goto('/blog')}>{t('app.blog.back_to_list', 'Back to Blog')}</Button>
			</div>
		</CardContent>
	</Card>
{:else}
	<form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-6">
		{#if error}
			<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
				{error}
			</div>
		{/if}
		{#if success}
			<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
				{success}
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
						bind:value={title}
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
						<AutocompleteSelect
							id="post-status"
							bind:value={status}
							options={statusOptions.length === 0
								? [{ value: 'draft', label: t('app.blog.status_draft', 'Draft') }]
								: statusOptions}
							class="w-full"
						/>
					</div>
					<div>
						<label for="post-visibility" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							{t('app.blog.field_visibility', 'Visibility')}
						</label>
						<AutocompleteSelect
							id="post-visibility"
							bind:value={visibility}
							options={visibilityOptions.length === 0
								? [{ value: 'public', label: t('app.blog.visibility_public', 'Public') }]
								: visibilityOptions}
							class="w-full"
						/>
					</div>
				</div>
			</CardContent>
		</Card>

		<div class="flex items-center justify-between">
			<div class="flex gap-2">
				{#if originalStatus === 'draft' || originalStatus === 'in_review'}
					<Button type="button" variant="outline" onclick={handlePublish} disabled={saving} style="color: var(--color-success)">
						<CheckCircle class="mr-2 h-4 w-4" />
						{t('app.blog.action_publish', 'Publish')}
					</Button>
				{:else if originalStatus === 'published'}
					<Button type="button" variant="outline" onclick={handleUnpublish} disabled={saving} style="color: var(--color-warning)">
						<XCircle class="mr-2 h-4 w-4" />
						{t('app.blog.action_unpublish', 'Unpublish')}
					</Button>
				{:else if originalStatus === 'archived'}
					<Button type="button" variant="outline" onclick={async () => {
						saving = true;
						try {
							const result = await blogAdminApi.restorePost(postId);
							if (result.ok) {
								status = 'draft';
								originalStatus = 'draft';
								success = t('app.blog.success_restored', 'Post restored successfully');
							} else {
								error = result.error;
							}
						} catch {
							error = t('app.blog.error_restore', 'Failed to restore post');
						} finally {
							saving = false;
						}
					}} disabled={saving} style="color: var(--color-info)">
						<RefreshCw class="mr-2 h-4 w-4" />
						{t('app.blog.action_restore', 'Restore')}
					</Button>
				{/if}
				{#if canManageBlog}
					<Button type="button" variant="destructive" onclick={handleDelete} disabled={saving}>
						<Trash2 class="mr-2 h-4 w-4" />
						{t('app.blog.delete', 'Delete')}
					</Button>
				{/if}
			</div>
			<div class="flex gap-2">
				<Button variant="ghost" onclick={() => goto('/blog')}>
					{t('app.blog.cancel', 'Cancel')}
				</Button>
				<Button type="submit" disabled={saving}>
					<Save class="mr-2 h-4 w-4" />
					{saving ? t('app.blog.saving', 'Saving...') : t('app.blog.save', 'Save')}
				</Button>
			</div>
		</div>
	</form>
{/if}

<!-- Delete confirmation dialog (replaces window.confirm per jc-admin-develop spec) -->
<Modal bind:open={showDeleteConfirm} title={t('app.blog.confirm_delete', 'Delete Post')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('app.blog.confirm_delete_body', 'Are you sure you want to delete this post? This action cannot be undone.')}
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)} disabled={saving}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
			<Trash2 class="mr-2 h-4 w-4" />
			{t('app.blog.delete', 'Delete')}
		</Button>
	</div>
</Modal>
