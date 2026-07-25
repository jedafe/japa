import { describe, it, expect, vi, beforeEach } from 'vitest';
import { DirtyForm } from './useDirtyForm.svelte';

type Res = { data: unknown; error: { message: string; status: number } | null; status: number };

const ok = (data: unknown = {}): Res => ({ data, error: null, status: 200 });
const fail = (status: number): Res => ({ data: null, error: { message: 'x', status }, status });

describe('DirtyForm', () => {
	let save: ReturnType<typeof vi.fn<(v: Record<string, unknown>) => Promise<Res>>>;
	beforeEach(() => (save = vi.fn<(v: Record<string, unknown>) => Promise<Res>>().mockResolvedValue(ok())));

	it('is not dirty until a value changes', () => {
		const f = new DirtyForm({ name: 'Jane', bio: '' }, save);
		expect(f.dirty).toBe(false);
		f.set({ name: 'Janet' });
		expect(f.dirty).toBe(true);
	});

	it('detects changes in NESTED objects (not just top-level)', () => {
		const f = new DirtyForm({ prefs: { email: true, push: false } }, save);
		expect(f.dirty).toBe(false);
		f.set({ prefs: { email: true, push: true } });
		expect(f.dirty).toBe(true);
	});

	it('save() persists then rebases the snapshot (dirty → false)', async () => {
		const f = new DirtyForm({ name: 'Jane' }, save);
		f.set({ name: 'Janet' });
		await f.save();
		expect(save).toHaveBeenCalledWith({ name: 'Janet' });
		expect(f.dirty).toBe(false);
	});

	it('reset() restores the snapshot', () => {
		const f = new DirtyForm({ name: 'Jane' }, save);
		f.set({ name: 'Janet' });
		f.reset();
		expect(f.values.name).toBe('Jane');
		expect(f.dirty).toBe(false);
	});

	it('classifies a 4xx save failure and stays dirty', async () => {
		const f = new DirtyForm({ name: 'Jane' }, vi.fn().mockResolvedValue(fail(422)));
		f.set({ name: '' });
		await f.save();
		expect(f.error?.cls).toBe('validation');
		expect(f.dirty).toBe(true);
	});
});
