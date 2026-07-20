export const FEATURE_ENVIRONMENTS = ['production', 'staging', 'development'] as const;

export type FeatureEnvironment = (typeof FEATURE_ENVIRONMENTS)[number];
