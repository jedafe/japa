import 'package:japaconnect_core/japaconnect_core.dart';

class JapaBotService {
  final JcApiClient _client;
  JapaBotService(this._client);

  Future<List<Map<String, dynamic>>> listConversations() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/ai/conversations',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
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
