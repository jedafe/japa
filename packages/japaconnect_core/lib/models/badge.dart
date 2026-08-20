class Badge {
  final String id;
  final String slug;
  final String name;
  final String? description;
  final String? triggerType;
  final int? triggerValue;
  final String? triggerActivityKey;
  final bool? isActive;
  final bool? isSystem;
  final int? displayOrder;
  final DateTime? createdAt;
  final DateTime? earnedAt;

  Badge({
    required this.id,
    required this.slug,
    required this.name,
    this.description,
    this.triggerType,
    this.triggerValue,
    this.triggerActivityKey,
    this.isActive,
    this.isSystem,
    this.displayOrder,
    this.createdAt,
    this.earnedAt,
  });

  factory Badge.fromJson(Map<String, dynamic> json) {
    return Badge(
      id: json['id'] as String,
      slug: json['slug'] as String,
      name: json['name'] as String,
      description: json['description'] as String?,
      triggerType: json['trigger_type'] as String?,
      triggerValue: json['trigger_value'] as int?,
      triggerActivityKey: json['trigger_activity_key'] as String?,
      isActive: json['is_active'] as bool?,
      isSystem: json['is_system'] as bool?,
      displayOrder: json['display_order'] as int?,
      createdAt: _parseDateTime(json['created_at']),
      earnedAt: _parseDateTime(json['earned_at']),
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
        'slug': slug,
        'name': name,
        if (description != null) 'description': description,
        if (triggerType != null) 'trigger_type': triggerType,
        if (triggerValue != null) 'trigger_value': triggerValue,
        if (triggerActivityKey != null) 'trigger_activity_key': triggerActivityKey,
        if (isActive != null) 'is_active': isActive,
        if (isSystem != null) 'is_system': isSystem,
        if (displayOrder != null) 'display_order': displayOrder,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (earnedAt != null) 'earned_at': earnedAt!.toIso8601String(),
      };
}
