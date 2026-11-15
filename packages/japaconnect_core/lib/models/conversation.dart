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

  // ConversationInfo other-party fields (§07 verification + 000942 org
  // conversations). The list endpoint returns these flat on the item —
  // `withUser` is a legacy shape kept for compatibility.
  final String? otherUsername;
  final String? otherDisplayName;
  final String? otherAvatarUrl;
  final bool otherVerified;
  final String? otherVerificationType;
  final String? conversationType;
  final String? orgSlug;

  Conversation({
    required this.id,
    this.title,
    this.withUser,
    this.lastMessageAt,
    this.otherUsername,
    this.otherDisplayName,
    this.otherAvatarUrl,
    this.otherVerified = false,
    this.otherVerificationType,
    this.conversationType,
    this.orgSlug,
  });

  factory Conversation.fromJson(Map<String, dynamic> json) {
    return Conversation(
      id: json['id'] as String,
      title: json['title'] as String?,
      withUser: json['with_user'] is Map<String, dynamic>
          ? User.fromJson(json['with_user'])
          : null,
      lastMessageAt: _parseDateTime(json['last_message_at']),
      otherUsername: json['other_username'] as String?,
      otherDisplayName: json['other_display_name'] as String?,
      otherAvatarUrl: json['other_avatar_url'] as String?,
      otherVerified: json['other_verified'] as bool? ?? false,
      otherVerificationType: json['other_verification_type'] as String?,
      conversationType: json['conversation_type'] as String?,
      orgSlug: json['org_slug'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        if (title != null) 'title': title,
        if (withUser != null) 'with_user': withUser!.toJson(),
        if (lastMessageAt != null) 'last_message_at': lastMessageAt!.toIso8601String(),
        if (otherUsername != null) 'other_username': otherUsername,
        if (otherDisplayName != null) 'other_display_name': otherDisplayName,
        if (otherAvatarUrl != null) 'other_avatar_url': otherAvatarUrl,
        'other_verified': otherVerified,
        if (otherVerificationType != null)
          'other_verification_type': otherVerificationType,
        if (conversationType != null) 'conversation_type': conversationType,
        if (orgSlug != null) 'org_slug': orgSlug,
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
