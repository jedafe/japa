import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/api/client', () => ({
	api: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn(), put: vi.fn() }
}));

import { api } from '$lib/api/client';
import { classifyError, createContent, searchHits, initiateUpload } from './create';

describe('classifyError', () => {
	it('classifies by status band', () => {
		expect(classifyError(0)).toBe('network');
		expect(classifyError(409)).toBe('conflict');
		expect(classifyError(422)).toBe('validation');
		expect(classifyError(400)).toBe('validation');
		expect(classifyError(500)).toBe('server');
		expect(classifyError(503)).toBe('server');
	});
});

describe('createContent', () => {
	beforeEach(() => vi.clearAllMocks());
	it('POSTs /content with the input body', async () => {
		(api.post as any).mockResolvedValue({ data: { id: 'c1', slug: 's' }, error: null, status: 201 });
		const res = await createContent({ content_type: 'blog_post', title: 'Hi' });
		expect(api.post).toHaveBeenCalledWith('/content', { content_type: 'blog_post', title: 'Hi' });
		expect(res.data).toEqual({ id: 'c1', slug: 's' });
	});
});

describe('searchHits', () => {
	beforeEach(() => vi.clearAllMocks());
	it('returns [] without calling the API below min length', async () => {
		const hits = await searchHits('users', 'a');
		expect(hits).toEqual([]);
		expect(api.get).not.toHaveBeenCalled();
	});
	it('maps user results on a valid query', async () => {
		(api.get as any).mockResolvedValue({
			data: [{ id: 'u1', display_name: 'Jane', username: 'jane' }],
			error: null,
			status: 200
		});
		const hits = await searchHits('users', 'jane');
		expect(api.get).toHaveBeenCalledWith('/search?type=user&q=jane');
		expect(hits[0]).toMatchObject({ id: 'u1', label: 'Jane', handle: 'jane' });
	});
});

describe('initiateUpload', () => {
	beforeEach(() => vi.clearAllMocks());
	it('POSTs /media/upload with the JSON initiate body (presigned-URL flow)', async () => {
		(api.post as any).mockResolvedValue({
			data: { media_id: 'm1', storage_path: 'temp/t/x.png', presigned_url: '/seaweedfs/upload/x', expires_at: '2099-01-01T00:00:00Z' },
			error: null,
			status: 201
		});
		const res = await initiateUpload({ filename: 'cover.png', mime_type: 'image/png', file_size: 12345 });
		expect(api.post).toHaveBeenCalledWith('/media/upload', {
			filename: 'cover.png',
			mime_type: 'image/png',
			file_size: 12345
		});
		expect(res.data?.media_id).toBe('m1');
		expect(res.data?.presigned_url).toBe('/seaweedfs/upload/x');
	});
});
