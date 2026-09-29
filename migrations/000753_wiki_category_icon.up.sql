-- wiki_type_categories.icon: optional lucide icon name for the category tab/
-- card, configured per type in the admin wiki categories table.
ALTER TABLE wiki_type_categories ADD COLUMN IF NOT EXISTS icon text;
