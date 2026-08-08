<script lang="ts">
	import { slide, fade } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { FileText, MessageSquare, PenTool, HelpCircle, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { compose } from './compose.svelte';
	import DiscussionComposer from './DiscussionComposer.svelte';

	/**
	 * Compose bottom sheet — ADR-058: the Compose button opens a composer
	 * with a content-type selector. Discussions and questions compose inline
	 * in the sheet (questions use the same editor with the question toggle
	 * pre-set); blog posts and CPTs route to /create.
	 *
	 * The sheet slides up from the bottom of the viewport (Flarum/Discuss
	 * pattern) rather than appearing as a centered dialog. Ref:
	 * japaconnect-engineering/adr/ADR-058-main-app-shell.md
	 */

	// Full-width options that route to /create.
	const routeOptions = $derived([
		{ key: 'blog', icon: PenTool, label: t('create.blog', 'Blog'), href: '/create?type=blog_post' },
		{ key: 'guide', icon: FileText, label: t('create.post', 'Guide'), href: '/create?type=guide' }
	]);

	function selectRoute(opt: (typeof routeOptions)[number]) {
		compose.close();
		goto(opt.href);
	}

	function onBackdropClick() {
		compose.close();
	}
</script>

{#if compose.open}
	<!-- Backdrop -->
	<button
		type="button"
		class="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm"
		aria-label={t('actions.close', 'Close')}
		onclick={onBackdropClick}
		transition:fade={{ duration: 200 }}
	></button>

	<!-- Bottom sheet — constrained to the AppShell center column via the
	     --current-nav-width / --sidebar-width vars (inherited from .shell).
	     Falls back to full width on tablet/mobile where the nav/sidebar drop. -->
	<div
		class="composer-sheet fixed bottom-0 left-[var(--current-nav-width)] right-[var(--sidebar-width)] z-50 max-h-[85vh] overflow-y-auto rounded-t-2xl border-t border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl"
		transition:slide={{ duration: 250, axis: 'y' }}
	>
		<!-- Drag handle -->
		<div class="sticky top-0 z-10 flex items-center justify-between rounded-t-2xl bg-[var(--color-surface)] px-4 pt-3 pb-2">
			<div class="mx-auto h-1 w-10 rounded-full bg-[var(--color-border)]"></div>
			<button
				type="button"
				onclick={() => compose.close()}
				class="absolute right-3 top-2.5 grid h-8 w-8 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('actions.close', 'Close')}
			>
				<X size={18} />
			</button>
		</div>

		<div class="px-4 pb-6">
			{#if compose.mode === 'discussion'}
				<DiscussionComposer
					onClose={() => compose.close()}
					initialIsQuestion={compose.isQuestion}
				/>
			{:else}
				<h2 class="mb-4 text-base font-medium text-[var(--color-text)]">
					{t('create.title', 'Create')}
				</h2>
				<div class="flex flex-col gap-2">
					<!-- Discussion + Question on the same row (Question pushed right) -->
					<div class="flex items-stretch gap-2">
						<button
							type="button"
							onclick={() => compose.openDiscussion(false)}
							class="flex flex-1 items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-left text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						>
							<MessageSquare size={20} class="text-[var(--color-primary)]" />
							<span>{t('create.discussion', 'Discussion')}</span>
						</button>
						<button
							type="button"
							onclick={() => compose.openDiscussion(true)}
							class="flex flex-1 items-center justify-end gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-right text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						>
							<span>{t('create.question', 'Question')}</span>
							<HelpCircle size={20} class="text-[var(--color-primary)]" />
						</button>
					</div>
					<!-- Full-width options that route to /create -->
					{#each routeOptions as opt (opt.key)}
						<button
							type="button"
							onclick={() => selectRoute(opt)}
							class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-left text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						>
							<opt.icon size={20} class="text-[var(--color-primary)]" />
							<span>{opt.label}</span>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	/* The sheet should not exceed the viewport; the content scrolls inside. */
	/* Match the AppShell breakpoints: the right sidebar drops below 1280px
	   and the left nav drops below 768px, so the sheet reclaims that space. */
	@media (max-width: 1279px) {
		.composer-sheet {
			right: 0;
		}
	}
	@media (max-width: 767px) {
		.composer-sheet {
			left: 0;
		}
	}
</style>
