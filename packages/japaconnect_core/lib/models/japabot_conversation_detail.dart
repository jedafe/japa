import 'japabot_message.dart';

class JapaBotConversationDetail {
  final String id;
  final int? turnCount;
  final List<JapaBotMessage> messages;

  JapaBotConversationDetail({
    required this.id,
    this.turnCount,
    this.messages = const [],
  });

  factory JapaBotConversationDetail.fromJson(Map<String, dynamic> json) {
    // ADR-130: unified threads return `messages` (ai_messages rows with
    // top-level role/content); legacy Honcho threads return `entries`
    // (context entries with role under metadata.role).
    final raw = (json['messages'] as List<dynamic>?) ??
        (json['entries'] as List<dynamic>?) ??
        const [];
    return JapaBotConversationDetail(
      id: (json['conversation_id'] ?? json['id'] ?? '') as String,
      turnCount: json['turn_count'] as int?,
      messages: raw
          .map((e) => JapaBotMessage.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Map<String, dynamic> toJson() => {
        'conversation_id': id,
        if (turnCount != null) 'turn_count': turnCount,
        'entries': messages.map((m) => m.toJson()).toList(),
      };
}
