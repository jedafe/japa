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
    final entries = json['entries'] as List<dynamic>? ?? [];
    return JapaBotConversationDetail(
      id: (json['conversation_id'] ?? json['id'] ?? '') as String,
      turnCount: json['turn_count'] as int?,
      messages: entries
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
