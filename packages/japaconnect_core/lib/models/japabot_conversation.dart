class JapaBotConversation {
  final String id;
  final String? title;
  final int? turnCount;
  final DateTime? lastEntryAt;

  JapaBotConversation({
    required this.id,
    this.title,
    this.turnCount,
    this.lastEntryAt,
  });

  factory JapaBotConversation.fromJson(Map<String, dynamic> json) {
    return JapaBotConversation(
      id: (json['conversation_id'] ?? json['id']) as String,
      title: (json['preview'] ?? json['title']) as String?,
      turnCount: json['turn_count'] as int?,
      lastEntryAt: _parseDateTime(json['last_entry_at'] ?? json['created_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  Map<String, dynamic> toJson() => {
        'conversation_id': id,
        if (title != null) 'preview': title,
        if (turnCount != null) 'turn_count': turnCount,
        if (lastEntryAt != null) 'last_entry_at': lastEntryAt!.toIso8601String(),
      };
}
