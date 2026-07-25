/**
 * Apply→call→rollback helper for discrete optimistic actions (toggles,
 * follows, deletes). `apply` runs synchronously before the network call;
 * on failure `rollback` restores the prior state.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Optimistic UI)
 */
interface OptimResult<T> {
	data: T | null;
	error: { code: string; message: string; status: number } | null;
	status: number;
}

export async function optimisticAction<T>(opts: {
	apply: () => void;
	rollback: () => void;
	call: () => Promise<OptimResult<T>>;
}): Promise<{ ok: boolean; data: T | null; error: OptimResult<T>['error'] }> {
	opts.apply();
	const res = await opts.call();
	if (res.error) {
		opts.rollback();
		return { ok: false, data: null, error: res.error };
	}
	return { ok: true, data: res.data, error: null };
}
