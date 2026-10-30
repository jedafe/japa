<script lang="ts">
	// MediaLibraryModal — main-app wrapper around the shared @japaconnect/ui
	// MediaPickerModal (ported from this file). Injects the app's media/stock
	// APIs and i18n; keeps the MediaAsset-typed onSelect contract unchanged.
	import { MediaPickerModal } from '@japaconnect/ui';
	import type { MediaPickerItem } from '@japaconnect/ui';
	import { t } from '$lib/i18n';
	import {
		listMyMedia,
		initiateUpload,
		uploadToPresigned,
		confirmMedia,
		getMedia
	} from '$lib/api/create';
	import { listStockProviders, searchStock, importStockPhoto } from '$lib/api/stock';
	import type { MediaAsset } from '$lib/types';

	interface Props {
		/** Called with the selected media when the user confirms. */
		onSelect: (media: MediaAsset) => void;
		/** Called when the modal is dismissed (cancel / overlay / X). */
		onClose: () => void;
		/** MIME type filter passed to /me/media?type=… (default: image). */
		allowedType?: string;
		/** Initial active tab when opening the modal. */
		initialTab?: 'library' | 'upload' | 'stock';
		/** Backend upload context — 'avatar' applies the avatar allowlist
		 * (users.allowed_avatar_types) instead of the general media one. */
		uploadContext?: 'media' | 'avatar' | 'document';
	}

	let { onSelect, onClose, allowedType = 'image', initialTab = 'library', uploadContext = 'media' }: Props = $props();
</script>

<MediaPickerModal
	{t}
	{allowedType}
	{initialTab}
	{uploadContext}
	fetchItems={async ({ type, q }) => {
		const res = await listMyMedia({ type, q });
		if (res.error || !res.data) throw new Error(res.error?.message ?? 'Failed to load media');
		return res.data;
	}}
	onUpload={async (file, context) => {
		const init = await initiateUpload({
			filename: file.name,
			mime_type: file.type || 'application/octet-stream',
			file_size: file.size,
			context
		});
		if (init.error || !init.data) throw new Error(init.error?.message ?? 'initiate failed');
		const up = await uploadToPresigned(init.data.presigned_url, file);
		if (!up.ok) throw new Error('upload failed');
		const conf = await confirmMedia(init.data.media_id);
		if (conf.error) throw new Error(conf.error.message);
		// confirmMedia returns { confirmed: true } and initiateUpload carries
		// no storage bucket, so fetch the serialized asset — its imgproxy URL
		// is canonical and avoids constructing a serve URL with the wrong
		// bucket (which rendered as a placeholder until the next reload).
		const media = await getMedia(init.data.media_id);
		return {
			id: init.data.media_id,
			url: media.data?.url ?? '',
			filename: file.name,
			mime_type: file.type,
			file_size: file.size,
			status: 'approved'
		};
	}}
	stock={{
		listProviders: async () => {
			const res = await listStockProviders();
			return res.data?.providers ?? [];
		},
		search: async (params) => {
			const res = await searchStock(params);
			if (res.error || !res.data) throw new Error(res.error?.message ?? 'Stock search failed');
			return res.data;
		},
		import: async (provider, id) => {
			const res = await importStockPhoto(provider, id);
			if (res.error || !res.data) throw new Error(res.error?.message ?? 'Import failed');
			return res.data;
		}
	}}
	onSelect={(item: MediaPickerItem) => onSelect(item as MediaAsset)}
	{onClose}
/>
