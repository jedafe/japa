/**
 * Shared feature-flag helpers over the root layout's GET /feature-flags
 * payload (features: Record<string, boolean>). Pure functions — importable
 * from both client components and server load functions.
 *
 * Ref: japaconnect-engineering/sections/42-feature-flags.md
 */

/**
 * The /messages/bookings surface gate (§111/000824): service_directory.enabled
 * is the primary flag, with service_packages.enabled as the fallback when the
 * directory flag is absent, and mentorship.enabled as an escape hatch —
 * mentorship bookings are served to the booker even when packages is off
 * (GET /me/service-bookings stays live on either flag). Both the route load
 * and the MessagesTabs visibility check share this so a visible tab never
 * lands on a 404 and vice versa.
 */
export function bookingsInboxEnabled(features: Record<string, boolean> | null | undefined): boolean {
	const f = features ?? {};
	return (
		(f['service_directory.enabled'] ?? f['service_packages.enabled'] ?? false) ||
		!!f['service_packages.enabled'] ||
		!!f['mentorship.enabled']
	);
}
