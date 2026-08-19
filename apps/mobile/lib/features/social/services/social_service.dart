import 'package:japaconnect_core/japaconnect_core.dart';

class SocialService {
  final JcApiClient _client;
  SocialService(this._client);

  Future<void> followUser(String userId) async {
    await _client.post<void>('/users/$userId/follow', parse: (_) {});
  }

  Future<void> unfollowUser(String userId) async {
    await _client.delete('/users/$userId/follow');
  }

  Future<List<Map<String, dynamic>>> listNotifications() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/me/notifications',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<void> markNotificationRead(String id) async {
    await _client.post<void>('/me/notifications/$id/read', parse: (_) {});
  }

  Future<void> markAllNotificationsRead() async {
    await _client.post<void>('/me/notifications/read-all', parse: (_) {});
  }

  Future<List<Map<String, dynamic>>> listConversations() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/dm/conversations',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<List<Map<String, dynamic>>> getConversationMessages(String id) async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/dm/conversations/$id/messages',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
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
