class Challenge {
  final String id;
  final String slug;
  final String name;
  final String? description;
  final String? imageMediaId;
  final String activityKey;
  final int targetCount;
  final int rewardPoints;
  final String? rewardBadgeId;
  final DateTime? startsAt;
  final DateTime? endsAt;
  final bool? isActive;
  final DateTime? createdAt;

  // Progress fields — populated by /me/challenges responses (PTS-007).
  final int? currentCount;
  final bool? completed;
  final DateTime? completedAt;

  Challenge({
    required this.id,
    required this.slug,
    required this.name,
    this.description,
    this.imageMediaId,
    this.activityKey = '',
    this.targetCount = 0,
    this.rewardPoints = 0,
    this.rewardBadgeId,
    this.startsAt,
    this.endsAt,
    this.isActive,
    this.createdAt,
    this.currentCount,
    this.completed,
    this.completedAt,
  });

  factory Challenge.fromJson(Map<String, dynamic> json) {
    return Challenge(
      id: json['id'] as String,
      slug: json['slug'] as String? ?? '',
      name: json['name'] as String,
      description: json['description'] as String?,
      imageMediaId: json['image_media_id'] as String?,
      activityKey: json['activity_key'] as String? ?? '',
      targetCount: json['target_count'] as int? ?? 0,
      rewardPoints: json['reward_points'] as int? ?? 0,
      rewardBadgeId: json['reward_badge_id'] as String?,
      startsAt: _parseDateTime(json['starts_at']),
      endsAt: _parseDateTime(json['ends_at']),
      isActive: json['is_active'] as bool?,
      createdAt: _parseDateTime(json['created_at']),
      currentCount: json['current_count'] as int?,
      completed: json['completed'] as bool?,
      completedAt: _parseDateTime(json['completed_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }
}
