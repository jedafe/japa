-- Section 10: Real-Time WebSockets + Direct Messaging

-- name: CreateWSTicket :one
INSERT INTO ws_tickets (user_id, expires_at) VALUES (@user_id, now() + interval '60 seconds')

RETURNING id, user_id, used, expires_at, created_at;

-- name: UseWSTicket :one
UPDATE ws_tickets SET used = true
WHERE id = @id AND user_id = @user_id AND used = false AND expires_at > now()
RETURNING *;

-- name: GetWSTicketForRedemption :one
-- Resolves a ticket by ID when the Valkey cache misses, returning the
-- user_id and the id needed to upgrade the WebSocket connection.
SELECT * FROM ws_tickets
WHERE id = @id AND used = false AND expires_at > now();

-- name: GetOrCreateConversation :one
-- Get existing conversation or create new one (ordering participant IDs)
INSERT INTO dm_conversations (participant_1_id, participant_2_id) VALUES (LEAST(@participant_1_id, @participant_2_id)::uuid, GREATEST(@participant_1_id, @participant_2_id)::uuid)

ON CONFLICT (participant_1_id, participant_2_id) DO UPDATE
SET updated_at = now()
RETURNING *;

-- name: GetConversationByID :one
SELECT * FROM dm_conversations WHERE id = @id;

-- name: GetConversationByParticipants :one
-- Find an existing conversation between two users without creating one.
-- Used by the DM privacy check (Section 48) to skip privacy enforcement
-- when a conversation already exists (existing threads stay active even
-- when dm_privacy is later set to 'nobody').
SELECT * FROM dm_conversations
WHERE participant_1_id = LEAST(@participant_1_id, @participant_2_id)::uuid
 AND participant_2_id = GREATEST(@participant_1_id, @participant_2_id)::uuid;

-- name: GetUserConversations :many
-- other_display_name / other_avatar_url are COALESCEd — the LEFT JOIN to
-- user_profiles yields NULL for users without a profile row, and scanning
-- NULL into a non-nullable string column 500s the whole list (BUG-024).
-- Optional params: @q searches username/display_name/last preview (ILIKE),
-- @filter 'unread' restricts to conversations with unread messages,
-- @sort 'recent'|'oldest'|'unread'|'name' picks the ORDER BY arm (the
-- CASE keys are all NULL for the unselected sorts, so they fall through).
-- Keyset cursor (Section 48): when @has_cursor is set, rows continue
-- strictly after the cursor position for the active sort — timestamp key
-- for recent/oldest, unread_count for 'unread', other-name for 'name' —
-- always tiebroken by dc.id. @page_offset only applies to cursor-less
-- (legacy) calls.
SELECT dc.id, dc.participant_1_id, dc.participant_2_id,
 dc.last_message_at, dc.last_message_preview,
 dc.p1_unread_count, dc.p2_unread_count, dc.created_at, dc.updated_at,
 (CASE WHEN dc.participant_1_id = @user_id THEN COALESCE(u2.public_username, u2.username) ELSE COALESCE(u1.public_username, u1.username) END)::text AS other_username,
 COALESCE(CASE WHEN dc.participant_1_id = @user_id THEN p2.display_name ELSE p1.display_name END,
          CASE WHEN dc.participant_1_id = @user_id THEN COALESCE(u2.public_username, u2.username) ELSE COALESCE(u1.public_username, u1.username) END)::text AS other_display_name,
 COALESCE(CASE WHEN dc.participant_1_id = @user_id THEN p2.avatar_url ELSE p1.avatar_url END, '')::text AS other_avatar_url,
 (CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END)::int AS unread_count
FROM dm_conversations dc
JOIN users u1 ON u1.id = dc.participant_1_id
JOIN users u2 ON u2.id = dc.participant_2_id
LEFT JOIN user_profiles p1 ON p1.user_id = u1.id
LEFT JOIN user_profiles p2 ON p2.user_id = u2.id
WHERE (dc.participant_1_id = @user_id OR dc.participant_2_id = @user_id)
 -- Section 48: a conversation backed by a PENDING dm_request stays out of
 -- the recipient's main inbox until they accept — it lives under Requests.
 AND NOT EXISTS (
   SELECT 1 FROM dm_requests dr
   WHERE dr.conversation_id = dc.id
    AND dr.recipient_id = @user_id
    AND dr.status = 'pending')
 AND (@filter::text <> 'unread' OR
      (CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END) > 0)
 AND (@q::text = '' OR
      (CASE WHEN dc.participant_1_id = @user_id THEN COALESCE(u2.public_username, u2.username) ELSE COALESCE(u1.public_username, u1.username) END) ILIKE '%' || @q || '%' OR
      COALESCE(CASE WHEN dc.participant_1_id = @user_id THEN p2.display_name ELSE p1.display_name END, '') ILIKE '%' || @q || '%' OR
      COALESCE(dc.last_message_preview, '') ILIKE '%' || @q || '%')
 AND (NOT @has_cursor::bool OR
      (@sort::text = 'recent' AND
        (COALESCE(dc.last_message_at, dc.created_at), dc.id) <
        (@cursor_ts::timestamptz, @cursor_id::uuid)) OR
      (@sort::text = 'oldest' AND
        (COALESCE(dc.last_message_at, dc.created_at), dc.id) >
        (@cursor_ts::timestamptz, @cursor_id::uuid)) OR
      (@sort::text = 'unread' AND
        ((CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END), dc.id) <
        (@cursor_int::bigint, @cursor_id::uuid)) OR
      (@sort::text = 'name' AND
        (COALESCE(CASE WHEN dc.participant_1_id = @user_id THEN p2.display_name ELSE p1.display_name END,
                  CASE WHEN dc.participant_1_id = @user_id THEN COALESCE(u2.public_username, u2.username) ELSE COALESCE(u1.public_username, u1.username) END), dc.id) >
        (@cursor_text::text, @cursor_id::uuid)))
ORDER BY
 (CASE WHEN @sort::text = 'unread' THEN
   (CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END) END) DESC NULLS LAST,
 (CASE WHEN @sort::text = 'unread' THEN dc.id END) DESC NULLS LAST,
 (CASE WHEN @sort::text = 'name' THEN
   COALESCE(CASE WHEN dc.participant_1_id = @user_id THEN p2.display_name ELSE p1.display_name END,
            CASE WHEN dc.participant_1_id = @user_id THEN COALESCE(u2.public_username, u2.username) ELSE COALESCE(u1.public_username, u1.username) END) END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'name' THEN dc.id END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'oldest' THEN COALESCE(dc.last_message_at, dc.created_at) END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'oldest' THEN dc.id END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'recent' THEN COALESCE(dc.last_message_at, dc.created_at) END) DESC NULLS LAST,
 (CASE WHEN @sort::text = 'recent' THEN dc.id END) DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: InsertDMMessage :exec
INSERT INTO dm_messages (conversation_id, sender_id, body) VALUES (@conversation_id, @sender_id, @body)
;

-- name: GetConversationMessages :many
-- Keyset-paginated by (created_at, id) DESC per Section 48 ("cursor-
-- paginated, keyset by created_at, newest first"). The cursor carries the
-- (created_at, id) of the previous page's last row; @page_offset only
-- applies to cursor-less (legacy) calls.
SELECT dm.*, COALESCE(u.public_username, u.username, '') AS sender_username, p.display_name AS sender_display_name
FROM dm_messages dm
JOIN users u ON u.id = dm.sender_id
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE dm.conversation_id = @conversation_id
 AND dm.deleted_at IS NULL
 AND (NOT @has_cursor::bool OR
      (dm.created_at, dm.id) < (@cursor_ts::timestamptz, @cursor_id::uuid))
ORDER BY dm.created_at DESC, dm.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MarkMessagesRead :exec
UPDATE dm_messages SET read_at = now()
WHERE conversation_id = @conversation_id
 AND sender_id != @user_id AND read_at IS NULL;

-- name: UpdateConversationLastMessage :exec
UPDATE dm_conversations
SET last_message_at = now(),
 last_message_preview = @last_message_preview,
 updated_at = now()
WHERE id = @conversation_id;

-- name: IncrementUnreadCount :exec
UPDATE dm_conversations
SET p1_unread_count = CASE WHEN participant_1_id = @user_id THEN p1_unread_count + 1 ELSE p1_unread_count END,
 p2_unread_count = CASE WHEN participant_2_id = @user_id THEN p2_unread_count + 1 ELSE p2_unread_count END
WHERE id = @conversation_id;

-- name: ResetUnreadCount :exec
UPDATE dm_conversations
SET p1_unread_count = CASE WHEN participant_1_id = @user_id THEN 0 ELSE p1_unread_count END,
 p2_unread_count = CASE WHEN participant_2_id = @user_id THEN 0 ELSE p2_unread_count END
WHERE id = @conversation_id;

-- name: GetMessageRequest :one
-- Looked up by the request's own id — the /dm/requests/{id}/* routes carry
-- the dm_requests.id, matching the id field in the list DTO (BUG-025:
-- previously keyed on conversation_id, so every action 500d).
SELECT * FROM dm_requests WHERE id = @id;

-- name: CreateMessageRequest :exec
INSERT INTO dm_requests (requester_id, recipient_id, conversation_id, preview_message, status) VALUES (@requester_id, @recipient_id, @conversation_id, @preview_message, 'pending')

ON CONFLICT (requester_id, recipient_id) DO NOTHING;

-- name: UpdateMessageRequestStatus :exec
UPDATE dm_requests SET status = @status, resolved_at = now()
WHERE id = @id;

-- name: GetPendingMessageRequests :many
-- Lists pending DM requests for a recipient (BUG-003). Filters by
-- recipient_id and status = 'pending' so the handler returns only
-- actionable requests, not all conversations.
SELECT dr.*, COALESCE(u.public_username, u.username, '') AS requester_username, p.display_name AS requester_display_name, p.avatar_url AS requester_avatar_url
FROM dm_requests dr
JOIN users u ON u.id = dr.requester_id
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE dr.recipient_id = @recipient_id AND dr.status = 'pending'
 AND (@q::text = '' OR u.username ILIKE '%' || @q || '%' OR
      u.public_username ILIKE '%' || @q || '%' OR
      COALESCE(p.display_name, '') ILIKE '%' || @q || '%' OR
      dr.preview_message ILIKE '%' || @q || '%')
 AND (NOT @has_cursor::bool OR
      (@sort::text = 'recent' AND
        (dr.created_at, dr.id) < (@cursor_ts::timestamptz, @cursor_id::uuid)) OR
      (@sort::text = 'oldest' AND
        (dr.created_at, dr.id) > (@cursor_ts::timestamptz, @cursor_id::uuid)) OR
      (@sort::text = 'name' AND
        (COALESCE(p.display_name, u.public_username, u.username, ''), dr.id) > (@cursor_text::text, @cursor_id::uuid)))
ORDER BY
 (CASE WHEN @sort::text = 'name' THEN COALESCE(p.display_name, u.public_username, u.username, '') END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'name' THEN dr.id END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'oldest' THEN dr.created_at END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'oldest' THEN dr.id END) ASC NULLS LAST,
 (CASE WHEN @sort::text = 'recent' THEN dr.created_at END) DESC NULLS LAST,
 (CASE WHEN @sort::text = 'recent' THEN dr.id END) DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: GetUserDMPrivacy :one
-- Scalar-subquery COALESCE always returns exactly one row — users without a
-- user_dm_settings row get 'everyone' instead of an ErrNoRows that used to
-- bubble up as a 500 on DM initiation (BUG-026).
SELECT COALESCE(
 (SELECT dm_privacy FROM user_dm_settings WHERE user_id = @user_id),
 'everyone'
)::text AS dm_privacy;

-- name: UpsertUserDMPrivacy :exec
INSERT INTO user_dm_settings (user_id, dm_privacy, updated_at) VALUES (@user_id, @dm_privacy, now())

ON CONFLICT (user_id) DO UPDATE SET dm_privacy = EXCLUDED.dm_privacy, updated_at = now();

-- name: EditDMMessage :exec
UPDATE dm_messages
SET body = @body, edited_at = now()
WHERE id = @id AND conversation_id = @conversation_id AND sender_id = @sender_id
 AND deleted_at IS NULL
 AND created_at > now() - interval '15 minutes';

-- name: SoftDeleteDMMessage :exec
UPDATE dm_messages
SET deleted_at = now(), body = ''
WHERE id = @id AND conversation_id = @conversation_id AND sender_id = @sender_id
 AND deleted_at IS NULL;

-- name: SoftDeleteDMMessageByID :exec
UPDATE dm_messages SET deleted_at = now(), body = ''
WHERE id = @id AND deleted_at IS NULL;

-- name: GetDMMessageByID :one
SELECT * FROM dm_messages WHERE id = @id AND conversation_id = @conversation_id;

-- name: CleanupDeclinedDMRequests :exec
DELETE FROM dm_requests
WHERE status = 'declined'
 AND updated_at < now() - interval '90 days';
