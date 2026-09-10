<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';

	interface SharePlatform {
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
	}

	interface Props {
		contentId: string;
		url: string;
		title: string;
		/** Called after a platform share (or copy) succeeds so the parent can show a transient "shared" state. */
		onShared?: () => void;
	}
	let { contentId, url, title, onShared }: Props = $props();

	let platforms = $state<SharePlatform[]>([]);

	$effect(() => {
		(async () => {
			const res = await api.get<SharePlatform[]>('/share-platforms/enabled');
			if (res.data) platforms = res.data;
		})();
	});

	function buildShareUrl(template: string): string {
		return template.replace('{url}', encodeURIComponent(url)).replace('{title}', encodeURIComponent(title));
	}

	async function shareToPlatform(platform: SharePlatform) {
		const shareUrl = buildShareUrl(platform.url_template);
		window.open(shareUrl, '_blank', 'noopener,noreferrer,width=600,height=500');
		onShared?.();
		await api.post(`/content/${contentId}/share`, {
			share_type: 'external',
			platform: platform.key
		});
	}
</script>

{#each platforms as platform (platform.key)}
	<DropdownMenu.Item
		class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
		onclick={() => shareToPlatform(platform)}
	>
		<span
			class="grid h-5 w-5 shrink-0 place-items-center rounded-full text-[10px] font-bold text-[var(--color-primary-contrast)]"
			style:backgroundColor={platform.color || 'var(--color-muted)'}
		>
			{platform.label.charAt(0)}
		</span>
		{platform.label}
	</DropdownMenu.Item>
{/each}
