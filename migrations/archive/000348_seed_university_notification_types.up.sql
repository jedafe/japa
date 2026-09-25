-- Section 36/39: Japa University notification types
-- The university service dispatches two notification types when instructor
-- applications are approved and when tracks are approved to the marketplace.
-- The notifications table has a FK to notification_types(key), so these must
-- be seeded before the dispatch code can insert rows.
-- Ref: japaconnect-engineering/sections/35-notification-system.md
-- Ref: japaconnect-engineering/sections/36-japa-university.md

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('university_instructor_approved', 'Instructor Application Approved',
     'Your instructor application has been approved',
     true, true, false),
    ('university_course_approved', 'Track Approved',
     'Your track has been approved for the marketplace',
     true, true, false)
ON CONFLICT (key) DO NOTHING;
