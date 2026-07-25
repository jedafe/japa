<script lang="ts">
	import { t } from '$lib/i18n';
	import { ImagePlus, Loader2 } from 'lucide-svelte';
	import { initiateUpload, uploadToPresigned, confirmMedia } from '$lib/api/create';
	import type { MediaAsset } from '$lib/types';

	interface Props {
		onUploaded: (asset: MediaAsset) => void;
	}

	let { onUploaded }: Props = $props();

	let busy = $state(false);
	let error = $state<string | null>(null);

	async function handleFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		busy = true;
		error = null;
		try {
			// Step 1: initiate — backend creates a media row + returns a presigned URL.
			const init = await initiateUpload({
				filename: file.name,
				mime_type: file.type,
				file_size: file.size
			});
			if (init.error || !init.data) throw new Error(init.error?.message ?? 'initiate failed');

			// Step 2: upload — PUT raw bytes directly to the presigned SeaweedFS URL.
			const up = await uploadToPresigned(init.data.presigned_url, file);
			if (!up.ok) throw new Error('upload failed');

			// Step 3: confirm — tell the backend the upload landed; get the canonical URL.
			const conf = await confirmMedia(init.data.media_id);
			if (conf.error || !conf.data) throw new Error(conf.error?.message ?? 'confirm failed');
			onUploaded(conf.data);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Upload failed';
		} finally {
			busy = false;
			input.value = '';
		}
	}
</script>

<div class="flex flex-col gap-1">
	<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.cover')}</label>
	<label
		class="flex cursor-pointer items-center justify-center gap-2 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-6 text-sm text-[var(--color-muted)] hover:border-[var(--color-primary)]"
	>
		{#if busy}
			<Loader2 size={18} class="animate-spin" />
			{t('create.status.uploading')}
		{:else}
			<ImagePlus size={18} />
			{t('create.placeholder.cover')}
		{/if}
		<input type="file" accept="image/*" class="hidden" onchange={handleFile} disabled={busy} />
	</label>
	{#if error}
		<span class="text-xs text-red-500">{error}</span>
	{/if}
</div>
