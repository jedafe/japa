class Bookmark {
  final String id;
  final String entityType;
  final String entityId;
  final String? title;
  final String? slug;
  final String? folder;
  final String? folderId;
  final DateTime? createdAt;

  Bookmark({
    required this.id,
    required this.entityType,
    required this.entityId,
    this.title,
    this.slug,
    this.folder,
    this.folderId,
    this.createdAt,
  });

  factory Bookmark.fromJson(Map<String, dynamic> json) {
    return Bookmark(
      id: json['id'] as String,
      entityType: json['entity_type'] as String,
      entityId: json['entity_id'] as String,
      title: json['title'] as String?,
      slug: json['slug'] as String?,
      folder: json['folder'] as String?,
      folderId: json['folder_id'] as String?,
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
        'entity_type': entityType,
        'entity_id': entityId,
        if (title != null) 'title': title,
        if (slug != null) 'slug': slug,
        if (folder != null) 'folder': folder,
        if (folderId != null) 'folder_id': folderId,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
