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

  Future<JapaBotConversationDetail> getConversation(String id) async {
    return _client.get<JapaBotConversationDetail>(
      '/ai/conversations/$id',
      parse: (json) => JapaBotConversationDetail.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<JapaBotMessage> sendMessage(
      String? conversationId, String message) async {
    final body = <String, dynamic>{'message': message};
    if (conversationId != null && conversationId.isNotEmpty) {
      body['conversation_id'] = conversationId;
    }
    return _client.post<JapaBotMessage>(
      '/ai/chat',
      body: body,
      parse: (json) => JapaBotMessage.fromJson(json as Map<String, dynamic>),
    );
  }
}
