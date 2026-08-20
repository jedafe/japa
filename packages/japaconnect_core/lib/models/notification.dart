class Notification {
  final String id;
  final String? title;
  final String? body;
  final bool? isRead;
  final DateTime? createdAt;

  Notification({
    required this.id,
    this.title,
    this.body,
    this.isRead,
    this.createdAt,
  });

  factory Notification.fromJson(Map<String, dynamic> json) {
    return Notification(
      id: json['id'] as String,
      title: json['title'] as String?,
      body: json['body'] as String?,
      isRead: json['is_read'] as bool?,
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
        if (body != null) 'body': body,
        if (isRead != null) 'is_read': isRead,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
