import type { LayoutServerLoad } from './$types';
import type { HealthStatus } from '$lib/types';

/**
 * Root layout server load.
 *
 * Placeholder for feature flags and a health check that will later be
 * resolved from the internal Go backend via `src/lib/server/api.ts`.
 */
export const load: LayoutServerLoad = async (): Promise<HealthStatus> => {
	// TODO: replace with a real API health probe once the Go service is wired up.
	return { healthy: true, flags: {} };
};