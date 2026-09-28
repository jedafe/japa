/// GET /me/notifications row (Section 35 — Notification System).
/// The backend emits `read_at` (nullable timestamp), `notification_type`,
/// `action_url`, and a joined `type_label`.
class Notification {
  final String id;
  final String? title;
  final String? body;
  final String? notificationType;
  final String? typeLabel;
  final String? actionUrl;
  final String? actorId;
  final DateTime? readAt;
  final DateTime? createdAt;

  Notification({
    required this.id,
    this.title,
    this.body,
    this.notificationType,
    this.typeLabel,
    this.actionUrl,
    this.actorId,
    this.readAt,
    this.createdAt,
  });

  /// Whether the notification has been read. Kept for backwards
  /// compatibility — the source of truth is [readAt].
  bool? get isRead => readAt != null;

  factory Notification.fromJson(Map<String, dynamic> json) {
    return Notification(
      id: json['id'] as String,
      title: json['title'] as String?,
      body: json['body'] as String?,
      notificationType: json['notification_type'] as String?,
      typeLabel: json['type_label'] as String?,
      actionUrl: json['action_url'] as String?,
      actorId: json['actor_id'] as String?,
      readAt: _parseDateTime(json['read_at'] ??
          // Legacy/fallback: older payloads may carry a boolean is_read.
          ((json['is_read'] as bool? ?? false) ? json['created_at'] : null)),
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
        if (notificationType != null) 'notification_type': notificationType,
        if (typeLabel != null) 'type_label': typeLabel,
        if (actionUrl != null) 'action_url': actionUrl,
        if (actorId != null) 'actor_id': actorId,
        if (readAt != null) 'read_at': readAt!.toIso8601String(),
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
