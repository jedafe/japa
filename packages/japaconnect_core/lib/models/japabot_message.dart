class JapaBotMessage {
  final String? id;
  final String text;
  final String? conversationId;
  final String? role;
  final DateTime? createdAt;
  final List<String> sources;

  JapaBotMessage({
    this.id,
    required this.text,
    this.conversationId,
    this.role,
    this.createdAt,
    this.sources = const [],
  });

  factory JapaBotMessage.fromJson(Map<String, dynamic> json) {
    return JapaBotMessage(
      id: json['id'] as String?,
      text: (json['reply'] ?? json['content'] ?? json['text'] ?? '') as String,
      conversationId: (json['conversation_id'] ?? json['conversationId']) as String?,
      role: (json['role'] ?? 'assistant') as String?,
      createdAt: _parseDateTime(json['created_at'] ?? json['createdAt']),
      sources: _parseSources(json['sources']),
    );
  }

  factory JapaBotMessage.user({
    required String text,
    String? conversationId,
    DateTime? createdAt,
  }) =>
      JapaBotMessage(
        text: text,
        conversationId: conversationId,
        role: 'user',
        createdAt: createdAt ?? DateTime.now(),
      );

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  static List<String> _parseSources(dynamic value) {
    if (value == null) return const [];
    if (value is List) {
      return value.whereType<String>().toList();
    }
    if (value is String) return [value];
    return const [];
  }

  Map<String, dynamic> toJson() => {
        if (id != null) 'id': id,
        'reply': text,
        if (conversationId != null) 'conversation_id': conversationId,
        if (role != null) 'role': role,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (sources.isNotEmpty) 'sources': sources,
      };
}
