<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { goto } from '$app/navigation';
	import { FileText, MessageSquare, PenTool, HelpCircle } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		open?: boolean;
	}

	let { open = $bindable(false) }: Props = $props();

	const options = $derived([
		{ href: '/create/post', icon: FileText, label: t('create.post', 'Post') },
		{ href: '/create/discussion', icon: MessageSquare, label: t('create.discussion', 'Discussion') },
		{ href: '/create/blog', icon: PenTool, label: t('create.blog', 'Blog') },
		{ href: '/create/question', icon: HelpCircle, label: t('create.question', 'Question') }
	]);

	function select(href: string) {
		open = false;
		goto(href);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
		/>
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-[90vw] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-xl"
		>
			<Dialog.Title class="mb-4 text-base font-medium text-[var(--color-text)]">
				{t('create.title', 'Create')}
			</Dialog.Title>
			<div class="flex flex-col gap-2">
				{#each options as opt (opt.href)}
					<button
						type="button"
						onclick={() => select(opt.href)}
						class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-left text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
					>
						<opt.icon size={20} class="text-[var(--color-primary)]" />
						<span>{opt.label}</span>
					</button>
				{/each}
			</div>
			<Dialog.Close
				class="absolute right-3 top-3 grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('actions.close', 'Close')}
			>
				✕
			</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
