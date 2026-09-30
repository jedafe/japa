<script lang="ts">
	import { MenuItem } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { shareContrastColor } from '$lib/utils/shareColor';

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
	<MenuItem onSelect={() => shareToPlatform(platform)}>
		<span
			class="grid h-5 w-5 shrink-0 place-items-center rounded-full text-[10px] font-bold"
			style:background-color={platform.color || 'var(--color-muted)'}
			style:color={platform.color ? shareContrastColor(platform.color) : 'var(--color-text)'}
		>
			{platform.label.charAt(0)}
		</span>
		{platform.label}
	</MenuItem>
{/each}
