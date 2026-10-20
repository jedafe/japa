-- 000914_next_action_sidebar_widget.down.sql
BEGIN;
DELETE FROM sidebar_widgets WHERE widget_type = 'next_action';
COMMIT;
