import 'package:japaconnect_core/utils/tiptap.dart';

class ForumThread {
  final String id;
  final String title;
  final String slug;
  /// Backing contents row id — needed for /content/{id}/report (§51).
  final String? contentId;
  final String? body;
  final String? bodyText;
  final int voteScore;
  final int viewCount;
  final int replyCount;
  final bool solved;
  final String? authorId;
  final String? authorUsername;
  final String? authorDisplayName;
  final String? authorAvatarUrl;
  final String? spaceId;
  final String? spaceName;
  final bool? isAnonymous;
  final bool? canEdit;
  final bool? canDelete;
  final bool? canSolve;
  final List<String>? tags;
  final List<ForumReply>? replies;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  ForumThread({
    required this.id,
    required this.title,
    required this.slug,
    this.contentId,
    this.body,
    this.bodyText,
    this.voteScore = 0,
    this.viewCount = 0,
    this.replyCount = 0,
    this.solved = false,
    this.authorId,
    this.authorUsername,
    this.authorDisplayName,
    this.authorAvatarUrl,
    this.spaceId,
    this.spaceName,
    this.isAnonymous,
    this.canEdit,
    this.canDelete,
    this.canSolve,
    this.tags,
    this.replies,
    this.createdAt,
    this.updatedAt,
  });

  factory ForumThread.fromJson(Map<String, dynamic> json) {
    final rawBody = json['body'] as String?;
    return ForumThread(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      contentId: json['content_id'] as String?,
      body: rawBody,
      bodyText: tiptapBase64ToText(rawBody),
      voteScore: (json['vote_score'] as num?)?.toInt() ?? 0,
      viewCount: (json['view_count'] as num?)?.toInt() ?? 0,
      replyCount: (json['reply_count'] as num?)?.toInt() ?? 0,
      solved: json['solved'] as bool? ?? false,
      authorId: json['author_user_id'] as String?,
      authorUsername: json['author_username'] as String?,
      authorDisplayName: json['author_display_name'] as String?,
      authorAvatarUrl: json['author_avatar_url'] as String?,
      spaceId: json['space_id'] as String?,
      spaceName: json['space_name'] as String?,
      isAnonymous: json['is_anonymous'] as bool?,
      canEdit: json['can_edit'] as bool?,
      canDelete: json['can_delete'] as bool?,
      canSolve: json['can_solve'] as bool?,
      tags: (json['tags'] as List?)?.map((e) => e.toString()).toList(),
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

  ForumThread copyWith({
    List<ForumReply>? replies,
  }) =>
      ForumThread(
        id: id,
        title: title,
        slug: slug,
        body: body,
        bodyText: bodyText,
        voteScore: voteScore,
        viewCount: viewCount,
        replyCount: replyCount,
        solved: solved,
        authorId: authorId,
        authorUsername: authorUsername,
        authorDisplayName: authorDisplayName,
        authorAvatarUrl: authorAvatarUrl,
        spaceId: spaceId,
        spaceName: spaceName,
        isAnonymous: isAnonymous,
        canEdit: canEdit,
        canDelete: canDelete,
        canSolve: canSolve,
        tags: tags,
        replies: replies ?? this.replies,
        createdAt: createdAt,
        updatedAt: updatedAt,
      );

  Map<String, dynamic> toCardMap() => {
        'title': title,
        'subtitle': bodyText ?? '',
        'avatar_url': authorAvatarUrl,
      };

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (body != null) 'body': body,
        if (bodyText != null) 'body_text': bodyText,
        'vote_score': voteScore,
        'view_count': viewCount,
        'reply_count': replyCount,
        'solved': solved,
        if (authorId != null) 'author_user_id': authorId,
        if (authorUsername != null) 'author_username': authorUsername,
        if (authorDisplayName != null) 'author_display_name': authorDisplayName,
        if (authorAvatarUrl != null) 'author_avatar_url': authorAvatarUrl,
        if (spaceId != null) 'space_id': spaceId,
        if (spaceName != null) 'space_name': spaceName,
        if (replies != null) 'replies': replies!.map((e) => e.toJson()).toList(),
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

class ForumReply {
  final String id;
  final String threadId;
  final String? parentReplyId;
  final String body;
  final String? bodyText;
  final int voteScore;
  final int viewCount;
  final bool isAnonymous;
  final bool isEdited;
  final String? authorId;
  final String? authorUsername;
  final String? authorDisplayName;
  final String? authorAvatarUrl;
  final bool? amaPending;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  ForumReply({
    required this.id,
    required this.threadId,
    this.parentReplyId,
    required this.body,
    this.bodyText,
    this.voteScore = 0,
    this.viewCount = 0,
    this.isAnonymous = false,
    this.isEdited = false,
    this.authorId,
    this.authorUsername,
    this.authorDisplayName,
    this.authorAvatarUrl,
    this.amaPending,
    this.createdAt,
    this.updatedAt,
  });

  factory ForumReply.fromJson(Map<String, dynamic> json) {
    final rawBody = json['body'] as String? ?? '';
    return ForumReply(
      id: json['id'] as String,
      threadId: json['thread_id'] as String,
      parentReplyId: json['parent_reply_id'] as String?,
      body: rawBody,
      bodyText: tiptapBase64ToText(rawBody),
      voteScore: (json['vote_score'] as num?)?.toInt() ?? 0,
      viewCount: (json['view_count'] as num?)?.toInt() ?? 0,
      isAnonymous: json['is_anonymous'] as bool? ?? false,
      isEdited: json['is_edited'] as bool? ?? false,
      authorId: json['author_user_id'] as String?,
      authorUsername: json['author_username'] as String?,
      authorDisplayName: json['author_display_name'] as String?,
      authorAvatarUrl: json['author_avatar_url'] as String?,
      amaPending: json['ama_pending'] as bool?,
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

  Map<String, dynamic> toJson() => {
        'id': id,
        'thread_id': threadId,
        if (parentReplyId != null) 'parent_reply_id': parentReplyId,
        'body': body,
        if (bodyText != null) 'body_text': bodyText,
        'vote_score': voteScore,
        'view_count': viewCount,
        'is_anonymous': isAnonymous,
        'is_edited': isEdited,
        if (authorId != null) 'author_user_id': authorId,
        if (authorUsername != null) 'author_username': authorUsername,
        if (authorDisplayName != null) 'author_display_name': authorDisplayName,
        if (authorAvatarUrl != null) 'author_avatar_url': authorAvatarUrl,
        if (amaPending != null) 'ama_pending': amaPending,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}
