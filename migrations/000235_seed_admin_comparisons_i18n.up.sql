-- 000235_seed_admin_comparisons_i18n.up.sql
-- BUG-006: the admin /comparisons page references i18n keys that were never
-- seeded (comparisons.col_*, vis_*, create, edit, view, body, type, type_none,
-- delete_confirm). Without these the admin t() returns the literal key string
-- as the column header. Seed both en and fr so the admin comparisons table
-- renders real labels. Idempotent: ON CONFLICT DO UPDATE so re-running is safe.

INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.col_title', 'Title'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.col_slug', 'Slug'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.col_visibility', 'Visibility'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.col_status', 'Status'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.col_created', 'Created'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.vis_public', 'Public'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.vis_members', 'Members'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.vis_private', 'Private'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.create', 'Create Comparison'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.edit', 'Edit Comparison'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.view', 'View Comparison'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.body', 'Body'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.type', 'Type'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.type_none', 'No type'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'comparisons.delete_confirm', 'Are you sure you want to delete this comparison?'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.col_title', 'Titre'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.col_slug', 'Slug'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.col_visibility', 'Visibilité'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.col_status', 'Statut'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.col_created', 'Créé le'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.vis_public', 'Public'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.vis_members', 'Membres'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.vis_private', 'Privé'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.create', 'Créer une comparaison'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.edit', 'Modifier la comparaison'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.view', 'Voir la comparaison'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.body', 'Corps'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.type', 'Type'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.type_none', 'Aucun type'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'comparisons.delete_confirm', 'Êtes-vous sûr de vouloir supprimer cette comparaison ?')
ON CONFLICT (tenant_id, locale, "key") DO UPDATE SET value = EXCLUDED.value, updated_at = now();
