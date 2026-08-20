import 'package:japaconnect_core/japaconnect_core.dart';

class JapaBotService {
  final JcApiClient _client;
  JapaBotService(this._client);

  Future<List<JapaBotConversation>> listConversations() async {
    return _client.get<List<JapaBotConversation>>(
      '/ai/conversations',
      parse: (json) =>
          (json as List).map((e) => JapaBotConversation.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Map<String, dynamic>> sendMessage(
      String conversationId, String message) async {
    return _client.post<Map<String, dynamic>>(
      '/ai/chat',
      body: {'conversation_id': conversationId, 'message': message},
      parse: (json) => json as Map<String, dynamic>,
    );
  }
}
