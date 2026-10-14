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
    // Role: unified ai_messages rows carry it top-level; legacy Honcho
    // context entries carry it under metadata.role.
    final role = (json['role'] ??
            (json['metadata'] as Map<String, dynamic>?)?['role'] ??
            'assistant') as String;
    return JapaBotMessage(
      id: json['id'] as String?,
      text: (json['reply'] ?? json['content'] ?? json['text'] ?? '') as String,
      conversationId: (json['conversation_id'] ?? json['conversationId']) as String?,
      role: role,
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
      // RAGChunk objects (ADR-130 sources event / JSON reply) carry
      // {title, url}; plain strings pass through.
      return value
          .map((e) {
            if (e is String) return e;
            if (e is Map<String, dynamic>) {
              return (e['title'] ?? e['url'] ?? '') as String;
            }
            return '';
          })
          .where((s) => s.isNotEmpty)
          .toList();
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
