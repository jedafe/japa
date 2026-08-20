class JapaBotConversation {
  final String id;
  final String? title;
  final DateTime? createdAt;

  JapaBotConversation({
    required this.id,
    this.title,
    this.createdAt,
  });

  factory JapaBotConversation.fromJson(Map<String, dynamic> json) {
    return JapaBotConversation(
      id: json['id'] as String,
      title: json['title'] as String?,
      createdAt: _parseDateTime(json['created_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        if (title != null) 'title': title,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
