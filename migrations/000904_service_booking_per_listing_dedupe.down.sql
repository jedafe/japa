DROP INDEX idx_svc_bookings_mentorship_open_uniq;
CREATE UNIQUE INDEX idx_svc_bookings_mentorship_open_uniq
    ON service_bookings (booker_user_id, provider_user_id)
    WHERE org_id IS NULL AND status IN ('pending', 'confirmed');
