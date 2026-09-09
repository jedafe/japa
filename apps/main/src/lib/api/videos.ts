import { api } from '$lib/api/client';
import type { VideoImportJob, VideoSource } from '$lib/types';

/**
 * §106 video — resolve/upload helpers for the /create composer's VideoField.
 * Both mutating endpoints return { source, embed_json } where embed_json is a
 * TipTap document containing a `video` node ready to append to the editor.
 */
export interface VideoResolveResult {
	source: VideoSource;
	/** TipTap JSON doc: { type: 'doc', content: [{ type: 'video', attrs: … }] }. */
	embed_json: Record<string, unknown>;
}

/** POST /api/v1/videos/resolve — resolve a public video URL (YouTube etc.). */
export const resolveVideo = (url: string) =>
	api.post<VideoResolveResult>('/videos/resolve', { url });

/** POST /api/v1/videos/upload — direct multipart upload of a video file. */
export const uploadVideo = (file: File, meta: { title?: string; description?: string } = {}) => {
	const fd = new FormData();
	fd.append('video', file);
	if (meta.title) fd.append('title', meta.title);
	if (meta.description) fd.append('description', meta.description);
	return api.postMultipart<VideoResolveResult>('/videos/upload', fd);
};

/** GET /api/v1/videos/{id} — public read of a video source. */
export const getVideoSource = (id: string) => api.get<VideoSource>(`/videos/${id}`);

/** Preview result for a bulk import dry-run. */
export interface BulkImportPreview {
	count: number;
	sample?: VideoSource;
	source_type: string;
	source_url?: string | null;
	query?: string | null;
}

/** POST /api/v1/me/video-imports/preview — dry-run bulk import preview. */
export const previewBulkImport = (body: {
	source_type: string;
	source_url?: string;
	query?: string;
	target_username?: string;
	auto_publish?: boolean;
}) => api.post<BulkImportPreview>('/me/video-imports/preview', body);

/** POST /api/v1/me/video-imports/{id}/retry — re-enqueue the user's import job. */
export const retryImportJob = (id: string) => api.post<VideoImportJob>(`/me/video-imports/${id}/retry`);
