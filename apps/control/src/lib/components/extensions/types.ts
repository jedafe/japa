// Shared types + constants for the /extensions catalog UI.
// Ref: internal/control/handlers.go (extensionEntry) and
// internal/features/features.go (SurfaceRule, Phases, Surfaces).
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §6.

// SurfaceRule is the per-surface visibility, audience, and geo policy stored
// inside the flag's config blob. A surface absent from the map behaves as
// {state:"visible"} — everyone, all countries.
export interface SurfaceRule {
	state: 'visible' | 'hidden' | 'disabled';
	audience?: string[]; // group:<slug>|perm:<node>; empty = everyone
	geo_allow?: string[]; // ISO 3166-1 alpha-2, uppercase; empty = all countries
	geo_deny?: string[]; // ISO alpha-2 denied even when also in geo_allow
}

// Extension is the per-item shape returned by GET /control/extensions — the
// union of features.Registry keys and every feature_flags row.
export interface Extension {
	key: string;
	name: string;
	description: string;
	enabled: boolean;
	version: string;
	setting_key: string;
	settings_category: string;
	category: string;
	rollout_percentage?: number;
	// phase/show_on_roadmap/surfaces/linked_roadmap_slug come from the
	// feature_flags config blob (Task 9); absent values derive from enabled.
	phase?: string;
	show_on_roadmap?: boolean;
	surfaces?: Record<string, SurfaceRule>;
	linked_roadmap_slug?: string;
}

// SURFACES is the canonical ordered surface list (features.Surfaces). The UI
// hardcodes the order for display only — audience/geo vocabularies are fetched.
export const SURFACES = [
	'main',
	'tools',
	'academy',
	'docs',
	'mobile',
	'admin',
	'extension',
	'desktop'
] as const;

export type SurfaceName = (typeof SURFACES)[number];

// PHASES is the rollout-phase vocabulary (features.Phases). Invalid values are
// rejected by the API with 400 invalid_phase.
export const PHASES = ['planned', 'in_progress', 'testing', 'shipped', 'archived'] as const;

export type PhaseName = (typeof PHASES)[number];

// SURFACE_STATES is the per-surface state vocabulary (ValidateConfig).
export const SURFACE_STATES = ['visible', 'hidden', 'disabled'] as const;

export type SurfaceState = (typeof SURFACE_STATES)[number];
