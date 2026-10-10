/**
 * Shared feature-flag helpers over the root layout's GET /feature-flags
 * payload (features: Record<string, boolean>). Pure functions — importable
 * from both client components and server load functions.
 *
 * Ref: japaconnect-engineering/sections/42-feature-flags.md
 */

/**
 * The /messages/bookings surface gate (§111/000824): the backend
 * GET /me/service-bookings route sits under
 * GateAny(service_directory.enabled, mentorship.enabled), so the inbox is
 * live whenever either flag is on — mentorship bookings are served to the
 * booker even when the directory is off. service_packages.enabled does not
 * keep the surface alive on its own: packages cannot outlive the directory.
 * Both the route load and the MessagesTabs visibility check share this so a
 * visible tab never lands on a 404 and vice versa.
 */
export function bookingsInboxEnabled(features: Record<string, boolean> | null | undefined): boolean {
	const f = features ?? {};
	return !!f['service_directory.enabled'] || !!f['mentorship.enabled'];
}
