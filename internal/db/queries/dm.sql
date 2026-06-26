-- Section 10: Real-Time WebSockets + Direct Messaging

-- name: CreateWSTicket :exec
INSERT INTO ws_tickets (user_id, tenant_id, expires_at)
VALUES (@user_id, @tenant_id, now() + interval '60 seconds');

-- name: UseWSTicket :one
UPDATE ws_tickets SET used = true
WHERE id = @id AND user_id = @user_id AND used = false AND expires_at > now()
RETURNING *;

-- name: GetOrCreateConversation :one
-- Get existing conversation or create new one (ordering participant IDs)
INSERT INTO dm_conversations (tenant_id, participant_1_id, participant_2_id)
VALUES (@tenant_id,
    LEAST(@participant_1_id, @participant_2_id),
    GREATEST(@participant_1_id, @participant_2_id))
ON CONFLICT (tenant_id, participant_1_id, participant_2_id) DO UPDATE
SET updated_at = now()
RETURNING *;

-- name: GetConversationByID :one
SELECT * FROM dm_conversations WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetUserConversations :many
SELECT dc.id, dc.tenant_id, dc.participant_1_id, dc.participant_2_id,
    dc.last_message_at, dc.last_message_preview,
    dc.p1_unread_count, dc.p2_unread_count, dc.created_at, dc.updated_at,
    (CASE WHEN dc.participant_1_id = @user_id THEN u2.username ELSE u1.username END)::text AS other_username,
    (CASE WHEN dc.participant_1_id = @user_id THEN p2.display_name ELSE p1.display_name END)::text AS other_display_name,
    (CASE WHEN dc.participant_1_id = @user_id THEN p2.avatar_url ELSE p1.avatar_url END)::text AS other_avatar_url,
    (CASE WHEN dc.participant_1_id = @user_id THEN dc.p1_unread_count ELSE dc.p2_unread_count END)::int AS unread_count
FROM dm_conversations dc
JOIN users u1 ON u1.id = dc.participant_1_id
JOIN users u2 ON u2.id = dc.participant_2_id
LEFT JOIN user_profiles p1 ON p1.user_id = u1.id AND p1.tenant_id = dc.tenant_id
LEFT JOIN user_profiles p2 ON p2.user_id = u2.id AND p2.tenant_id = dc.tenant_id
WHERE dc.tenant_id = @tenant_id
  AND (dc.participant_1_id = @user_id OR dc.participant_2_id = @user_id)
ORDER BY dc.last_message_at DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: InsertDMMessage :exec
INSERT INTO dm_messages (conversation_id, sender_id, body)
VALUES (@conversation_id, @sender_id, @body);

-- name: GetConversationMessages :many
SELECT dm.*, u.username AS sender_username, p.display_name AS sender_display_name
FROM dm_messages dm
JOIN users u ON u.id = dm.sender_id
LEFT JOIN user_profiles p ON p.user_id = u.id AND p.tenant_id = @tenant_id
WHERE dm.conversation_id = @conversation_id
ORDER BY dm.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MarkMessagesRead :exec
UPDATE dm_messages SET read_at = now()
WHERE conversation_id = @conversation_id AND sender_id != @user_id AND read_at IS NULL;

-- name: UpdateConversationLastMessage :exec
UPDATE dm_conversations
SET last_message_at = @last_message_at,
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
SELECT * FROM dm_message_requests WHERE conversation_id = @conversation_id;

-- name: CreateMessageRequest :exec
INSERT INTO dm_message_requests (conversation_id, status)
VALUES (@conversation_id, 'pending')
ON CONFLICT (conversation_id) DO NOTHING;

-- name: UpdateMessageRequestStatus :exec
UPDATE dm_message_requests SET status = @status WHERE conversation_id = @conversation_id;

-- name: GetUserDMPrivacy :one
SELECT COALESCE(dm_privacy, 'everyone') AS dm_privacy FROM user_dm_settings WHERE user_id = @user_id;

-- name: UpsertUserDMPrivacy :exec
INSERT INTO user_dm_settings (user_id, dm_privacy, updated_at)
VALUES (@user_id, @dm_privacy, now())
ON CONFLICT (user_id) DO UPDATE SET dm_privacy = EXCLUDED.dm_privacy, updated_at = now();
