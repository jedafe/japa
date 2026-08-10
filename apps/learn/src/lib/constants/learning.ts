/**
 * Shared constants for the Learn app.
 *
 * Difficulty levels are a fixed vocabulary backed by the `learning_tracks.difficulty`
 * text column (default 'beginner'). They are NOT admin-configurable — there is no
 * system_settings key or API endpoint that returns them — so a single source-of-truth
 * constant is the correct pattern (Rule 16 forbids hardcoded *configurable* data;
 * schema-level enums are not configurable).
 *
 * Ref: §36 learning platform — learning_tracks.difficulty column.
 */

export const DIFFICULTY_LEVELS = ['beginner', 'intermediate', 'advanced'] as const;
export type DifficultyLevel = (typeof DIFFICULTY_LEVELS)[number];
