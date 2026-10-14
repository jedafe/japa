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

  /// Streams a unified Japa AI turn (ADR-130): POST /ai/chat with
  /// `Accept: text/event-stream`. Yields the raw agent events — `delta`
  /// chunks render progressively; `final_response` carries the complete
  /// reply; `conversation` adopts the thread id; `sources` carries RAG
  /// citations; `error` marks failure.
  Stream<Map<String, dynamic>> streamMessage({
    String? conversationId,
    required String message,
    bool? toolsEnabled,
    String? model,
    String? voiceId,
  }) {
    final body = <String, dynamic>{'message': message};
    if (conversationId != null &&
        conversationId.isNotEmpty &&
        conversationId != 'new') {
      body['conversation_id'] = conversationId;
    }
    if (toolsEnabled != null) {
      body['tools'] = {'enabled': toolsEnabled};
    }
    if (model != null && model.isNotEmpty) body['model'] = model;
    if (voiceId != null && voiceId.isNotEmpty) body['voice_id'] = voiceId;
    return _client.streamSse('/ai/chat', body: body);
  }

  /// Returns the tool catalog this user may invoke (empty list → the chat
  /// surface shows no tools controls; turns stay pure chat).
  Future<List<Map<String, dynamic>>> getCapabilities() async {
    final data = await _client.get<Map<String, dynamic>>(
      '/ai/agent/capabilities',
      parse: (json) => json as Map<String, dynamic>,
    );
    final tools = data['tools'] as List<dynamic>? ?? const [];
    return tools.whereType<Map<String, dynamic>>().toList();
  }

  /// Best-effort cancel for an in-flight agent session (stops the loop at
  /// its next iteration check).
  Future<void> cancelSession(String sessionId) async {
    await _client.post<void>(
      '/ai/agent/sessions/$sessionId/cancel',
      parse: (_) {},
    );
  }
}
