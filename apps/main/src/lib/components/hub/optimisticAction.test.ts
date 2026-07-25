import { describe, it, expect, vi } from 'vitest';
import { optimisticAction } from './optimisticAction';

describe('optimisticAction', () => {
	it('applies and keeps the change on success', async () => {
		const apply = vi.fn(), rollback = vi.fn();
		const call = vi.fn().mockResolvedValue({ data: { id: 'x' }, error: null, status: 200 });
		const res = await optimisticAction({ apply, rollback, call });
		expect(apply).toHaveBeenCalledOnce();
		expect(rollback).not.toHaveBeenCalled();
		expect(res.ok).toBe(true);
		expect(res.data).toEqual({ id: 'x' });
	});
	it('rolls back on error', async () => {
		const apply = vi.fn(), rollback = vi.fn();
		const call = vi.fn().mockResolvedValue({ data: null, error: { code: 'e', message: 'no', status: 403 }, status: 403 });
		const res = await optimisticAction({ apply, rollback, call });
		expect(rollback).toHaveBeenCalledOnce();
		expect(res.ok).toBe(false);
		expect(res.error?.status).toBe(403);
	});
});
