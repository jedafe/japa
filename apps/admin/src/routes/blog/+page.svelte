<script lang="ts">
	// /blog — migrated to shared components (ActionMenu, FilterPanel, SlideOver,
	// bulkActionsList). Tabbed page with state actions (publish/unpublish/restore/
	// reject/delete); keeps page-local Modals for reject (form input) and delete
	// (confirmation). View detail opens in a SlideOver.
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import { FileText, Plus, RefreshCw, Trash2, ExternalLink, CheckCircle, XCircle } from 'lucide-svelte';
	import { api, blogAdminApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type RawBlogPost = {
		id: string;
		title: string;
		slug: string;
		author_id: string;
		author_name?: string;
		status: string;
		content_type?: string;
		body?: string;
		body_html?: string;
		field_values?: string | null;
		published_at?: string | null;
		publish_at?: string | null;
		created_at: string;
		updated_at?: string;
		view_count?: number;
		views?: number;
		excerpt?: string;
		categories?: string[];
		tags?: string[];
		url?: string;
		canonical_url?: string | null;
	};

	type BlogPost = {
		id: string;
		title: string;
		slug: string;
		author_id: string;
		author_name: string;
		status: string;
		categories: string[];
		tags: string[];
		published_at?: string;
		created_at: string;
		excerpt?: string;
		views?: number;
		url?: string;
	};

	function mapBlogPost(raw: RawBlogPost): BlogPost {
		let categories: string[] = raw.categories ?? [];
		let tags: string[] = raw.tags ?? [];
		if (raw.field_values) {
			try {
				const fv = JSON.parse(raw.field_values);
				if (Array.isArray(fv.categories)) categories = fv.categories;
				if (Array.isArray(fv.tags)) tags = fv.tags;
			} catch { /* ignore */ }
		}
		let excerpt = raw.excerpt;
		if (!excerpt && raw.body) {
			try {
				const parsed = JSON.parse(raw.body);
				const texts: string[] = [];
				const extract = (node: any) => {
					if (texts.join(' ').length > 160) return;
					if (node.type === 'text' && node.text) {
						const clean = node.text.trim();
						if (clean && clean !== '/') texts.push(clean);
					}
					if (node.content) node.content.forEach(extract);
				};
				if (parsed.content) parsed.content.slice(0, 5).forEach(extract);
				const joined = texts.join(' ').trim();
				excerpt = joined.substring(0, 160);
				if (joined.length > 160) excerpt += '…';
			} catch {
				const clean = raw.body.trim();
				if (clean && clean !== '/') excerpt = clean.substring(0, 160);
			}
		}
		return {
			id: raw.id,
			title: raw.title || raw.slug,
			slug: raw.slug,
			author_id: raw.author_id,
			author_name: raw.author_name || '—',
			status: raw.status || 'draft',
			categories,
			tags,
			published_at: raw.published_at ?? raw.publish_at ?? (raw.status === 'published' ? raw.created_at : undefined),
			created_at: raw.created_at,
			excerpt: excerpt || undefined,
			views: raw.views ?? raw.view_count ?? 0,
			url: raw.url || (raw.slug ? `/blog/${raw.slug}` : undefined)
		};
	}

	let posts = $state<BlogPost[]>([]);
	let postsLoaded = $state(false);
	let error = $state('');
	let activeTab = $state('all');
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());

	// View SlideOver
	let showViewSlideOver = $state(false);
	let viewTarget = $state<BlogPost | null>(null);

	// Reject Modal (page-local — has form input)
	let showRejectModal = $state(false);
	let rejectTarget = $state<BlogPost | null>(null);
	let rejectReason = $state('');
	let acting = $state(false);

	// Delete Modal (page-local confirmation)
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<BlogPost | null>(null);

	let bulkDeleting = $state(false);

	const canManageBlog = $derived(hasPermission('blog.review.manage'));

	const tabs = $derived([
		{ value: 'all', label: `${t('app.blog.tab_all', 'All Posts')} (${posts.length})` },
		{ value: 'published', label: `${t('app.blog.tab_published', 'Published')} (${posts.filter((p) => p.status === 'published').length})` },
		{ value: 'draft', label: `${t('app.blog.tab_drafts', 'Drafts')} (${posts.filter((p) => p.status === 'draft').length})` },
		{ value: 'in_review', label: `${t('app.blog.tab_in_review', 'In Review')} (${posts.filter((p) => p.status === 'in_review').length})` },
		{ value: 'archived', label: `${t('app.blog.tab_archived', 'Archived')} (${posts.filter((p) => p.status === 'archived').length})` }
	]);

	const columns = [
		{ key: 'title', label: t('app.blog.col_title', 'Title') },
		{ key: 'author_name', label: t('app.blog.col_author', 'Author'), render: (v: string) => v || t('app.blog.unknown', 'Unknown') },
		{
			key: 'tags',
			label: t('app.blog.col_tags', 'Tags'),
			render: (v: string[]) => {
				if (!v || v.length === 0) return '—';
				return v.slice(0, 2).join(', ') + (v.length > 2 ? ` +${v.length - 2}` : '');
			}
		},
		{
			key: 'status',
			label: t('app.blog.col_status', 'Status'),
			render: (v: string) => {
				const colors: Record<string, string> = { published: 'var(--color-success)', draft: 'var(--color-warning)', in_review: 'var(--color-info)', review: 'var(--color-info)', archived: 'var(--color-muted)' };
				const labels: Record<string, string> = { published: 'published', draft: 'draft', in_review: 'in review', review: 'in review', archived: 'archived' };
				const color = colors[v] || colors.draft;
				const label = labels[v] || v || 'draft';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, ${color} 15%, transparent); color: ${color}">${label}</span>`;
			}
		},
		{ key: 'published_at', label: t('app.blog.col_published', 'Published'), render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') },
		{ key: 'views', label: t('app.blog.col_views', 'Views'), render: (v: number) => (v ?? 0).toLocaleString() }
	];

	// Tag filter options derived from loaded posts (Rule 16 — no hardcoded data).
	const tagOptions = $derived.by(() => {
		const tags = new Set<string>();
		for (const p of posts) if (p.tags) for (const tg of p.tags) tags.add(tg);
		return Array.from(tags).sort().map((tg) => ({ value: tg, label: tg }));
	});

	const filters = $derived(tagOptions.length > 0 ? [{ key: 'tag', label: t('app.blog.filter_tag', 'Tag'), options: tagOptions }] : []);

	let filteredPosts = $derived.by(() => {
		let result = [...posts];
		if (activeTab !== 'all') result = result.filter((p) => p.status === activeTab);
		if (filterValue.tag) result = result.filter((p) => p.tags?.includes(filterValue.tag));
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			result = result.filter((p) => p.title?.toLowerCase().includes(q) || p.author_name?.toLowerCase().includes(q) || p.tags?.some((t) => t.toLowerCase().includes(q)));
		}
		return result;
	});

	// ── Row actions via ActionMenu ──────────────────────────────────────────
	const rowActions: ActionItem[] = [
		{ label: t('app.blog.action_view', 'View'), onclick: (row) => openView(row) },
		{ label: t('app.blog.action_edit', 'Edit'), onclick: (row: BlogPost) => goto(`/blog/${row.slug || row.id}`) },
		{
			label: t('app.blog.action_publish', 'Publish'),
			icon: CheckCircle,
			onclick: (row) => handlePublish(row),
			show: (row: BlogPost) => row.status === 'draft' || row.status === 'in_review'
		},
		{
			label: t('app.blog.action_unpublish', 'Unpublish'),
			icon: XCircle,
			onclick: (row) => handleUnpublish(row),
			show: (row: BlogPost) => row.status === 'published'
		},
		{
			label: t('app.blog.action_restore', 'Restore'),
			onclick: (row) => handleRestore(row),
			show: (row: BlogPost) => row.status === 'archived'
		},
		{
			label: t('app.blog.reject', 'Reject'),
			variant: 'destructive',
			onclick: (row) => openReject(row),
			show: (row: BlogPost) => row.status === 'in_review' && canManageBlog
		},
		{
			label: t('app.blog.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDelete(row),
			show: () => canManageBlog
		}
	];

	// ── Bulk actions via BulkBar ────────────────────────────────────────────
	const bulkActions: BulkAction[] = [
		{ label: t('app.blog.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
	];

	async function loadPosts() {
		postsLoaded = false;
		error = '';
		try {
			const result = await api<RawBlogPost[]>('/admin/blog');
			postsLoaded = true;
			if (result.ok) {
				posts = ((result.data as RawBlogPost[]) ?? []).map(mapBlogPost);
			} else {
				error = result.error;
			}
		} catch {
			postsLoaded = true;
			error = t('app.blog.error_load', 'Failed to load blog posts');
		}
	}

	function openView(post: BlogPost) {
		viewTarget = post;
		showViewSlideOver = true;
	}

	function openReject(post: BlogPost) {
		rejectTarget = post;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!rejectTarget) return;
		acting = true;
		error = '';
		try {
			const result = await blogAdminApi.rejectPost(rejectTarget.id, { reason: rejectReason });
			if (result.ok) {
				showRejectModal = false;
				loadPosts();
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_reject', 'Failed to reject post');
		} finally {
			acting = false;
		}
	}

	function confirmDelete(post: BlogPost) {
		deleteTarget = post;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await blogAdminApi.deletePost(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				loadPosts();
			} else {
				error = result.error;
			}
		} catch {
			error = t('app.blog.error_delete', 'Failed to delete post');
		} finally {
			acting = false;
		}
	}

	async function handlePublish(post: BlogPost) {
		acting = true;
		error = '';
		try {
			const result = await blogAdminApi.publishPost(post.id);
			if (result.ok) loadPosts();
			else error = result.error;
		} catch {
			error = t('app.blog.error_publish', 'Failed to publish post');
		} finally {
			acting = false;
		}
	}

	async function handleUnpublish(post: BlogPost) {
		acting = true;
		error = '';
		try {
			const result = await blogAdminApi.unpublishPost(post.id);
			if (result.ok) loadPosts();
			else error = result.error;
		} catch {
			error = t('app.blog.error_unpublish', 'Failed to unpublish post');
		} finally {
			acting = false;
		}
	}

	async function handleRestore(post: BlogPost) {
		acting = true;
		error = '';
		try {
			const result = await blogAdminApi.restorePost(post.id);
			if (result.ok) loadPosts();
			else error = result.error;
		} catch {
			error = t('app.blog.error_restore', 'Failed to restore post');
		} finally {
			acting = false;
		}
	}

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		error = '';
		try {
			const results = await Promise.all(idArr.map((id) => api(`/admin/blog/${id}`, { method: 'DELETE' })));
			const failed = results.filter((r) => !r.ok);
			if (failed.length > 0) {
				error = t('app.blog.error_bulk_delete_partial', '{n} of {total} deletes failed').replace('{n}', String(failed.length)).replace('{total}', String(idArr.length));
			}
		} catch {
			error = t('app.blog.error_bulk_delete', 'Failed to delete selected posts');
		} finally {
			bulkDeleting = false;
		}
	}

	$effect(() => {
		loadPosts();
	});
</script>

<svelte:head>
	<title>{t('app.blog.page_title', 'Blog — {site} Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.blog.title', 'Blog Posts')}
	description={t('app.blog.description', 'Manage blog content and review submissions')}
	actions={[
		{ label: t('app.blog.new_post', 'New Post'), icon: Plus, onclick: () => goto('/blog/new') },
		{ label: t('app.blog.review_queue', 'Review Queue'), icon: FileText, onclick: () => goto('/blog/review-queue') },
		{ label: t('app.blog.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: loadPosts }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

	{#if error}
		<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<Card>
		<CardContent class="p-0">
			{#if !postsLoaded}
				<TableSkeleton rows={8} columns={columns.length} />
			{:else}
				<DataTable
					columns={columns}
					data={filteredPosts}
					selectable
					bind:selectedIds
					pageSize={25}
					emptyMessage={t('app.blog.empty_message', 'No blog posts found')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					{filters}
					bind:filterValue
					actions={rowActions}
					bulkActionsList={bulkActions}
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('app.blog.modal_post_details', 'Post Details')} size="lg">
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_title', 'Title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
			</div>
			{#if viewTarget.excerpt}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_excerpt', 'Excerpt')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.excerpt}</p>
				</div>
			{/if}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_author', 'Author')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.author_name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_views', 'Views')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.views ?? 0).toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_published', 'Published')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.published_at ? new Date(viewTarget.published_at).toLocaleString() : '—'}</p>
				</div>
			</div>
			{#if viewTarget.tags?.length}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_tags', 'Tags')}</p>
					<div class="mt-1 flex flex-wrap gap-1">
						{#each viewTarget.tags as tag (tag)}
							<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background: var(--color-surface); color: var(--color-muted)">{tag}</span>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => goto(`/blog/${viewTarget?.slug || viewTarget?.id}`)}>{t('app.blog.detail_edit', 'Edit')}</Button>
			{#if viewTarget?.url}
				<Button variant="outline" onclick={() => viewTarget?.url && window.open(viewTarget.url, '_blank')}>
					<ExternalLink class="me-1 h-3.5 w-3.5 rtl:-scale-x-100" /> {t('app.blog.view_live', 'View Live')}
				</Button>
			{/if}
		</div>
	{/snippet}
</SlideOver>

<!-- Reject Modal (page-local — has form input) -->
<Modal bind:open={showRejectModal} title={t('app.blog.modal_reject_post', 'Reject Post')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.blog.reject_confirm', 'Are you sure you want to reject "{title}"? The author will be notified.').replace('{title}', rejectTarget?.title ?? '')}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.blog.reason_optional', 'Reason (optional)')}</label>
			<textarea id="reject-reason" bind:value={rejectReason} rows="3" class="w-full rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t('app.blog.reject_placeholder', 'Explain why this post was rejected...')}></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.blog.rejecting', 'Rejecting...') : t('app.blog.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t('app.blog.modal_delete_post', 'Delete Post')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.blog.delete_confirm', 'Are you sure you want to delete "{title}"? This will soft-delete the post.').replace('{title}', deleteTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>
			{acting ? t('app.blog.deleting', 'Deleting...') : t('app.blog.delete', 'Delete')}
		</Button>
	</div>
</Modal>
