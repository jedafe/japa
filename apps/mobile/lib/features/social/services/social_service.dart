import 'package:japaconnect_core/japaconnect_core.dart';

class SocialService {
  final JcApiClient _client;
  SocialService(this._client);

  Future<void> followUser(String userId) async {
    await _client.post<void>('/users/$userId/follow', parse: (_) {});
  }

  // §41 user blocking (BLK-019). POST/DELETE /users/{id}/block are
  // idempotent server-side and return {blocked: bool}; block-status
  // returns {blocked, blocked_by} for the directional pair.
  Future<void> blockUser(String userId) async {
    await _client.post<void>('/users/$userId/block', parse: (_) {});
  }

  Future<void> unblockUser(String userId) async {
    await _client.delete('/users/$userId/block');
  }

  Future<bool> isBlocked(String userId) async {
    final data = await _client.get<Map<String, dynamic>>(
      '/users/$userId/block-status',
      parse: (json) => (json as Map).cast<String, dynamic>(),
    );
    return data['blocked'] == true;
  }

  Future<List<Map<String, dynamic>>> listBlockedUsers() async {
    final data = await _client.get<List<Map<String, dynamic>>>(
      '/me/blocked-users?page_size=100',
      parse: (json) =>
          (json as List).map((e) => (e as Map).cast<String, dynamic>()).toList(),
    );
    return data;
  }

  Future<void> unfollowUser(String userId) async {
    await _client.delete('/users/$userId/follow');
  }

  Future<List<Notification>> listNotifications() async {
    final response = await _client.get<List<Notification>>(
      // Max server page size (§35) — the screen has no pagination, so
      // without this the list silently caps at the 20-row default.
      '/me/notifications?page_size=100',
      parse: (json) =>
          (json as List).map((e) => Notification.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

  Future<void> markNotificationRead(String id) async {
    await _client.post<void>('/me/notifications/$id/read', parse: (_) {});
  }

  Future<void> markAllNotificationsRead() async {
    await _client.post<void>('/me/notifications/read-all', parse: (_) {});
  }

  Future<List<Conversation>> listConversations() async {
    final response = await _client.get<List<Conversation>>(
      '/dm/conversations',
      parse: (json) =>
          (json as List).map((e) => Conversation.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

  Future<List<Message>> getConversationMessages(String id) async {
    final response = await _client.get<List<Message>>(
      '/dm/conversations/$id/messages',
      parse: (json) =>
          (json as List).map((e) => Message.fromJson(e as Map<String, dynamic>)).toList(),
    );
    return response;
  }

  Future<void> sendConversationMessage(String id, String text) async {
    await _client.post<void>(
      '/dm/conversations/$id/messages',
      body: {'text': text},
      parse: (_) {},
    );
  }

  Future<void> markConversationRead(String id) async {
    await _client.post<void>('/dm/conversations/$id/read', parse: (_) {});
  }
}
