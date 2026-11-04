<script lang="ts">
	// Revision history (§44 amendment 5) — editor surface for viewing,
	// restoring, deleting, and clearing doc page revisions.
	import { untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import { goto } from '$app/navigation';
	import { confirmDialog } from '@japaconnect/ui';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { ArrowLeft, ChevronRight, Eye, History, RotateCcw, Trash2 } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	let confirmingClear = $state(false);
	let busy = $state(false);

	const pagePath = $derived(`/docs/${data.audience}/${data.slug}`);
	const basePath = $derived(`${pagePath}/history`);

	const total = $derived(data.revMeta?.total ?? data.revisions.length);
	const totalPages = $derived(Math.max(1, Math.ceil(total / data.revLimit)));

	function pageHref(p: number): string {
		const usp = new URLSearchParams();
		if (p > 1) usp.set('page', String(p));
		if (data.revLimit !== 20) usp.set('limit', String(data.revLimit));
		const qs = usp.toString();
		return qs ? `${basePath}?${qs}` : basePath;
	}

	const limitOptions = $derived(
		['10', '20', '50', '100'].map((v) => ({ value: v, label: `${v} / page` }))
	);

	function onLimitChange(v: string) {
		const usp = new URLSearchParams();
		if (v !== '20') usp.set('limit', v);
		const qs = usp.toString();
		goto(qs ? `${basePath}?${qs}` : basePath);
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}
</script>

<svelte:head>
	<SeoHead meta={data.meta} gated={true} />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience={data.audience}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<div class="history-page">
		<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
			<a href={pagePath} class="crumb"><ArrowLeft size={12} /> {data.page.title}</a>
			<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
			<span class="crumb-current">{t('history.title', 'Revision history')}</span>
		</nav>

		<header class="history-header">
			<h1>{t('history.title', 'Revision history')}</h1>
			{#if data.revisions.length > 0}
				<button
					type="button"
					class="clear-btn"
					onclick={() => (confirmingClear = true)}
				>
					<Trash2 size={14} />
					{t('history.clear_all', 'Clear history')}
				</button>
			{/if}
		</header>

		{#if form?.error}
			<p class="form-error" role="alert">{form.error}</p>
		{/if}

		{#if confirmingClear}
			<div class="confirm-bar" role="alert">
				<p>{t('history.clear_confirm', 'Clear all history for this page?')}</p>
				<form
					method="POST"
					action="?/clear"
					use:enhance={() => {
						busy = true;
						return async ({ update }) => {
							await update();
							busy = false;
							confirmingClear = false;
						};
					}}
				>
					<input type="hidden" name="page_id" value={data.page.id} />
					<button type="submit" class="danger-btn" disabled={busy}>
						{t('history.clear_all', 'Clear history')}
					</button>
				</form>
				<button type="button" class="ghost-btn" onclick={() => (confirmingClear = false)}>
					{t('editor.cancel', 'Cancel')}
				</button>
			</div>
		{/if}

		{#if data.revisions.length === 0}
			<p class="empty">
				<History size={18} />
				{t('history.empty', 'No revisions yet.')}
			</p>
		{:else}
			<ol class="rev-list">
				{#each data.revisions as rev (rev.id)}
					<li class="rev-item">
						<div class="rev-main">
							<div class="rev-top">
								<span class="rev-version">
									{t('history.version', 'v{n}', { n: String(rev.version) })}
								</span>
								{#if rev.is_latest}
									<span class="latest-chip">{t('history.latest', 'latest')}</span>
								{/if}
								<span class="rev-title">{rev.title}</span>
							</div>
							{#if rev.summary}
								<p class="rev-summary">{rev.summary}</p>
							{/if}
							<p class="rev-meta">
								{formatDate(rev.created_at)}
								{#if rev.excerpt}· {rev.excerpt}{/if}
							</p>
						</div>
						<div class="rev-actions">
							<form
								method="POST"
								action="?/view"
								use:enhance={() => {
									busy = true;
									return async ({ update }) => {
										await update();
										busy = false;
									};
								}}
							>
								<input type="hidden" name="revision_id" value={rev.id} />
								<button type="submit" class="action-btn" disabled={busy}>
									<Eye size={13} /> {t('history.view', 'View')}
								</button>
							</form>
							<form
								method="POST"
								action="?/restore"
								use:enhance={async ({ cancel }) => {
									if (!(await confirmDialog(t('history.restore_confirm', 'Restore this version?')))) {
										cancel();
										return;
									}
									busy = true;
									return async ({ update }) => {
										await update();
										busy = false;
									};
								}}
							>
								<input type="hidden" name="page_id" value={data.page.id} />
								<input type="hidden" name="revision_id" value={rev.id} />
								<button type="submit" class="action-btn" disabled={busy}>
									<RotateCcw size={13} /> {t('history.restore', 'Restore')}
								</button>
							</form>
							{#if !rev.is_latest}
								<form
									method="POST"
									action="?/deleteRevision"
									use:enhance={async ({ cancel }) => {
										if (!(await confirmDialog(t('history.delete_confirm', 'Delete this revision?')))) {
											cancel();
											return;
										}
										busy = true;
										return async ({ update }) => {
											await update();
											busy = false;
										};
									}}
								>
									<input type="hidden" name="revision_id" value={rev.id} />
									<button type="submit" class="action-btn danger" disabled={busy}>
										<Trash2 size={13} /> {t('history.delete', 'Delete')}
									</button>
								</form>
							{/if}
						</div>
						{#if form?.viewId === rev.id && form.viewHtml}
							<div class="rev-view">
								<h2 class="rev-view-title">
									{form.viewTitle}
									<span class="rev-version">
										{t('history.version', 'v{n}', { n: String(form.viewVersion) })}
									</span>
								</h2>
								<div class="prose">
									<DocContent
										pageId={data.page.id}
										audience={data.audience}
										slug={data.slug}
										html={form.viewHtml}
									/>
								</div>
							</div>
						{/if}
					</li>
				{/each}
			</ol>

			<footer class="pager">
				<div class="pager-nav">
					{#if data.revPage > 1}
						<a href={pageHref(data.revPage - 1)} class="pager-link">← {t('page.previous', 'Previous')}</a>
					{/if}
					<span class="pager-status">{data.revPage} / {totalPages}</span>
					{#if data.revPage < totalPages}
						<a href={pageHref(data.revPage + 1)} class="pager-link">{t('page.next', 'Next')} →</a>
					{/if}
				</div>
				<AutocompleteSelect
					value={String(data.revLimit)}
					options={limitOptions}
					onValueChange={onLimitChange}
				/>
			</footer>
		{/if}
	</div>
</AppShell>

<style>
	.history-page {
		max-width: 820px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		margin-bottom: 1.25rem;
	}
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.crumb:hover { color: var(--color-primary); }
	.breadcrumb :global(.crumb-sep) { color: var(--color-muted); }
	.crumb-current { color: var(--color-text-subtle); font-weight: 500; }
	.history-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	.history-header h1 {
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0;
	}
	.clear-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.4375rem 0.875rem;
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-danger, #dc2626);
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
	}
	.clear-btn:hover {
		background: color-mix(in srgb, var(--color-danger, #dc2626) 8%, transparent);
	}
	.confirm-bar {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		margin-bottom: 1.25rem;
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		border-radius: var(--radius-card);
		background: color-mix(in srgb, var(--color-danger, #dc2626) 6%, transparent);
	}
	.confirm-bar p {
		flex: 1;
		min-width: 16rem;
		margin: 0;
		font-size: 0.875rem;
		color: var(--color-text);
	}
	.danger-btn {
		padding: 0.4375rem 1rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-danger, #dc2626);
		color: #fff;
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
	}
	.ghost-btn {
		padding: 0.4375rem 0.875rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text);
		font-size: 0.8125rem;
		cursor: pointer;
	}
	.form-error {
		padding: 0.75rem 1rem;
		margin: 0 0 1rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		color: var(--color-danger, #dc2626);
		font-size: 0.875rem;
	}
	.empty {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 2rem;
		justify-content: center;
		color: var(--color-muted);
		font-size: 0.9375rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-card);
	}
	.rev-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.rev-item {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		padding: 0.875rem 1rem;
	}
	.rev-main { min-width: 0; }
	.rev-top {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.rev-version {
		font-size: 0.6875rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
	}
	.latest-chip {
		font-size: 0.625rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--method-get, #16a34a);
		background: color-mix(in srgb, var(--method-get, #16a34a) 10%, transparent);
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
	}
	.rev-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.rev-summary {
		margin: 0.375rem 0 0;
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		font-style: italic;
	}
	.rev-meta {
		margin: 0.375rem 0 0;
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.rev-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.625rem;
		flex-wrap: wrap;
	}
	.rev-actions form { display: inline; }
	.action-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.3125rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.75rem;
		cursor: pointer;
	}
	.action-btn:hover:not(:disabled) { border-color: var(--color-primary); color: var(--color-primary); }
	.action-btn.danger:hover:not(:disabled) {
		border-color: var(--color-danger, #dc2626);
		color: var(--color-danger, #dc2626);
	}
	.action-btn:disabled { opacity: 0.5; cursor: default; }
	.rev-view {
		margin-top: 0.875rem;
		padding-top: 0.875rem;
		border-top: 1px solid var(--color-border);
	}
	.rev-view-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 1rem;
		margin: 0 0 0.75rem;
		color: var(--color-text);
	}
	.pager {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-top: 1.5rem;
	}
	.pager-nav {
		display: flex;
		align-items: center;
		gap: 0.875rem;
	}
	.pager-link {
		font-size: 0.8125rem;
		color: var(--color-primary);
		text-decoration: none;
	}
	.pager-status {
		font-size: 0.75rem;
		color: var(--color-muted);
		font-variant-numeric: tabular-nums;
	}
</style>
