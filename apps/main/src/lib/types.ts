/**
 * Shared TypeScript types for the main site.
 * Keep payload shapes here so client and server code agree on them.
 */

export interface FeatureFlags {
	[flag: string]: boolean;
}

export interface HealthStatus {
	healthy: boolean;
	flags: FeatureFlags;
}