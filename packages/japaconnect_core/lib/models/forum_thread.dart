class ForumThread {
  final String id;
  final String title;
  final String slug;
  final String? body;
  final List<ForumReply>? replies;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  ForumThread({
    required this.id,
    required this.title,
    required this.slug,
    this.body,
    this.replies,
    this.createdAt,
    this.updatedAt,
  });

  factory ForumThread.fromJson(Map<String, dynamic> json) {
    return ForumThread(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      body: json['body'] as String?,
      replies: (json['replies'] as List?)
          ?.map((e) => ForumReply.fromJson(e as Map<String, dynamic>))
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

  Map<String, dynamic> toCardMap() => {
        'title': title,
        'subtitle': '',
        'avatar_url': null,
      };

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (body != null) 'body': body,
        if (replies != null) 'replies': replies!.map((e) => e.toJson()).toList(),
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

class ForumReply {
  final String id;
  final String body;
  final String? authorId;
  final DateTime? createdAt;

  ForumReply({
    required this.id,
    required this.body,
    this.authorId,
    this.createdAt,
  });

  factory ForumReply.fromJson(Map<String, dynamic> json) {
    return ForumReply(
      id: json['id'] as String,
      body: json['body'] as String,
      authorId: json['author_user_id'] as String? ?? json['author_id'] as String?,
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
        'body': body,
        if (authorId != null) 'author_id': authorId,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
