class Content {
  final String id;
  final String title;
  final String slug;
  final String contentType;
  final String authorId;
  final String? authorName;
  final String? spaceId;
  final String? spaceName;
  final String? spaceSlug;
  final String? body;
  final String? excerpt;
  final int? readingTimeMinutes;
  final int? viewCount;
  final int? commentCount;
  final int? voteScore;
  final List<String>? tags;
  final List<ComparisonItem>? items;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  Content({
    required this.id,
    required this.title,
    required this.slug,
    required this.contentType,
    required this.authorId,
    this.authorName,
    this.spaceId,
    this.spaceName,
    this.spaceSlug,
    this.body,
    this.excerpt,
    this.readingTimeMinutes,
    this.viewCount,
    this.commentCount,
    this.voteScore,
    this.tags,
    this.items,
    this.createdAt,
    this.updatedAt,
  });

  factory Content.fromJson(Map<String, dynamic> json) {
    return Content(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      contentType: json['content_type'] as String,
      authorId: json['author_id'] as String,
      authorName: json['author_name'] as String?,
      spaceId: json['space_id'] as String?,
      spaceName: json['space_name'] as String?,
      spaceSlug: json['space_slug'] as String?,
      body: json['body'] as String?,
      excerpt: json['excerpt'] as String?,
      readingTimeMinutes: json['reading_time_minutes'] as int?,
      viewCount: json['view_count'] as int?,
      commentCount: json['comment_count'] as int?,
      voteScore: json['vote_score'] as int?,
      tags: (json['tags'] as List?)?.cast<String>(),
      items: (json['items'] as List?)
          ?.map((e) => ComparisonItem.fromJson(e as Map<String, dynamic>))
          .toList(),
      createdAt: _parseDateTime(json['created_at']),
      updatedAt: _parseDateTime(json['updated_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  String? get _subtitleSource => excerpt ?? body;

  String? get _subtitle {
    final src = _subtitleSource;
    if (src == null || src.isEmpty) return null;
    if (src.length <= 120) return src;
    return '${src.substring(0, 120).trimRight()}...';
  }

  /// Map representation for generic card widgets that expect
  /// `title`, `subtitle` and `avatar_url` keys.
  Map<String, dynamic> toCardMap() => {
        'title': title,
        'subtitle': _subtitle ?? '',
        'avatar_url': null,
      };

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        'content_type': contentType,
        'author_id': authorId,
        if (authorName != null) 'author_name': authorName,
        if (spaceId != null) 'space_id': spaceId,
        if (spaceName != null) 'space_name': spaceName,
        if (spaceSlug != null) 'space_slug': spaceSlug,
        if (body != null) 'body': body,
        if (excerpt != null) 'excerpt': excerpt,
        if (readingTimeMinutes != null) 'reading_time_minutes': readingTimeMinutes,
        if (viewCount != null) 'view_count': viewCount,
        if (commentCount != null) 'comment_count': commentCount,
        if (voteScore != null) 'vote_score': voteScore,
        if (tags != null) 'tags': tags,
        if (items != null) 'items': items!.map((e) => e.toJson()).toList(),
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

class ComparisonItem {
  final String id;
  final String label;
  final String value;

  ComparisonItem({
    required this.id,
    required this.label,
    required this.value,
  });

  factory ComparisonItem.fromJson(Map<String, dynamic> json) {
    return ComparisonItem(
      id: json['id'] as String,
      label: json['label'] as String,
      value: json['value'] as String,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'label': label,
        'value': value,
      };
}
