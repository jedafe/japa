import 'package:japaconnect_core/api/generated/lib/api.dart' show User;

DateTime? _parseDateTime(dynamic value) {
  if (value == null) return null;
  if (value is DateTime) return value;
  if (value is String) return DateTime.tryParse(value);
  return null;
}

class Conversation {
  final String id;
  final String? title;
  final User? withUser;
  final DateTime? lastMessageAt;

  Conversation({
    required this.id,
    this.title,
    this.withUser,
    this.lastMessageAt,
  });

  factory Conversation.fromJson(Map<String, dynamic> json) {
    return Conversation(
      id: json['id'] as String,
      title: json['title'] as String?,
      withUser: json['with_user'] is Map<String, dynamic>
          ? User.fromJson(json['with_user'])
          : null,
      lastMessageAt: _parseDateTime(json['last_message_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        if (title != null) 'title': title,
        if (withUser != null) 'with_user': withUser!.toJson(),
        if (lastMessageAt != null) 'last_message_at': lastMessageAt!.toIso8601String(),
      };
}

class Message {
  final String id;
  final String text;
  final String? senderId;
  final DateTime? createdAt;

  Message({
    required this.id,
    required this.text,
    this.senderId,
    this.createdAt,
  });

  factory Message.fromJson(Map<String, dynamic> json) {
    return Message(
      id: json['id'] as String,
      text: json['text'] as String,
      senderId: json['sender_id'] as String?,
      createdAt: _parseDateTime(json['created_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'text': text,
        if (senderId != null) 'sender_id': senderId,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
