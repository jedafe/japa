import { api } from '$lib/api/client';

/**
 * Two-step presigned-URL media upload (§34) — ported from the main app's
 * create.ts. POST /media/upload returns {media_id, presigned_url}; the file
 * bytes go straight to SeaweedFS via PUT (self-authorizing URL — no auth
 * header, no JSON envelope); POST /media/confirm/{id} finalizes the row.
 *
 * Used by the curriculum editor to attach video/document/audio files to
 * lessons (academy_lessons.media_id).
 */

export interface InitiateUploadInput {
	filename: string;
	mime_type: string;
	file_size: number;
	/** 'media' (default) covers video/audio; 'document' covers pdf/doc/etc. */
	context?: 'media' | 'document' | 'avatar';
}

export interface InitiateUploadResult {
	media_id: string;
	storage_path: string;
	presigned_url: string;
	expires_at: string;
}

export const initiateUpload = (input: InitiateUploadInput) =>
	api.post<InitiateUploadResult>('/media/upload', input);

export const confirmMedia = (id: string) =>
	api.post<{ confirmed: boolean }>(`/media/confirm/${id}`);

/** PUT the raw file bytes to the presigned URL (outside the api client). */
async function uploadToPresigned(presignedUrl: string, file: File): Promise<boolean> {
	try {
		const res = await fetch(presignedUrl, { method: 'PUT', body: file });
		return res.ok;
	} catch {
		return false;
	}
}

/**
 * Full upload pipeline — returns the confirmed media_id or an error message.
 * `context` picks the backend MIME allowlist ('document' for pdf/doc files).
 */
export async function uploadLessonFile(
	file: File,
	context: 'media' | 'document' = 'media'
): Promise<{ mediaId: string | null; error: string | null }> {
	const init = await initiateUpload({
		filename: file.name,
		mime_type: file.type || 'application/octet-stream',
		file_size: file.size,
		context
	});
	if (init.error || !init.data) {
		return { mediaId: null, error: init.error?.message ?? 'Upload initiation failed' };
	}

	const uploaded = await uploadToPresigned(init.data.presigned_url, file);
	if (!uploaded) {
		return { mediaId: null, error: 'File transfer failed' };
	}

	const conf = await confirmMedia(init.data.media_id);
	if (conf.error) {
		return { mediaId: null, error: conf.error.message };
	}

	return { mediaId: init.data.media_id, error: null };
}
