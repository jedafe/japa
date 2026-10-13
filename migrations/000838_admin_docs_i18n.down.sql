-- 000838_admin_docs_i18n down: remove the seeded admin docs i18n keys.
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'docs.approve', 'docs.back_to_docs', 'docs.cancel', 'docs.categories_catalog',
    'docs.categories_catalog_desc', 'docs.categories_desc',
    'docs.categories_no_permission', 'docs.categories_saved',
    'docs.categories_title', 'docs.category', 'docs.category_ph',
    'docs.confirm_delete', 'docs.create', 'docs.edit_doc_page', 'docs.generate',
    'docs.generate_accepted', 'docs.generate_with_ai', 'docs.generating',
    'docs.generation_prompt', 'docs.new_doc_page', 'docs.no_matches',
    'docs.parent_page', 'docs.parent_ph', 'docs.position', 'docs.publish',
    'docs.related_features', 'docs.saved_transition_failed', 'docs.saving',
    'docs.subcategory', 'docs.subcategory_ph', 'docs.subcategory_pick_cat',
    'docs.subcategory_ph_pick', 'docs.unpublish', 'docs.update',
    'settings.add_category', 'settings.add_subcategory', 'settings.category_desc_ph',
    'settings.category_icon', 'settings.category_name_ph',
    'settings.subcategory_name_ph'
  );

COMMIT;
