/// Japa Comparison Engine (JCE) data models.
///
/// Ports the TypeScript types in `apps/main/src/lib/types/jce.ts` to Dart.
class ListMeta {
  final int? total;
  final int? page;
  final int? limit;
  final List<int>? pageSizeOptions;
  final List<String>? sortOptions;

  const ListMeta({
    this.total,
    this.page,
    this.limit,
    this.pageSizeOptions,
    this.sortOptions,
  });

  factory ListMeta.fromJson(Map<String, dynamic> json) {
    return ListMeta(
      total: _parseInt(json['total']),
      page: _parseInt(json['page']),
      limit: _parseInt(json['limit']),
      pageSizeOptions: _parseIntList(json['page_size_options']),
      sortOptions: _parseStringList(json['sort_options']),
    );
  }

  Map<String, dynamic> toJson() => {
        if (total != null) 'total': total,
        if (page != null) 'page': page,
        if (limit != null) 'limit': limit,
        if (pageSizeOptions != null) 'page_size_options': pageSizeOptions,
        if (sortOptions != null) 'sort_options': sortOptions,
      };
}

class PaginatedList<T> {
  final List<T> items;
  final ListMeta? meta;

  const PaginatedList({required this.items, this.meta});
}

class ComparisonTypeConfig {
  final int? maxItemCount;
  final int? defaultItemCount;
  final List<String>? itemRefTypes;
  final String? defaultItemRefType;
  final bool? allowNestedComparisons;
  final List<String>? allowedNestedTypes;

  const ComparisonTypeConfig({
    this.maxItemCount,
    this.defaultItemCount,
    this.itemRefTypes,
    this.defaultItemRefType,
    this.allowNestedComparisons,
    this.allowedNestedTypes,
  });

  factory ComparisonTypeConfig.fromJson(Map<String, dynamic> json) {
    return ComparisonTypeConfig(
      maxItemCount: _parseInt(json['max_item_count']),
      defaultItemCount: _parseInt(json['default_item_count']),
      itemRefTypes: _parseStringList(json['item_ref_types']),
      defaultItemRefType: json['default_item_ref_type'] as String?,
      allowNestedComparisons: json['allow_nested_comparisons'] as bool?,
      allowedNestedTypes: _parseStringList(json['allowed_nested_types']),
    );
  }

  Map<String, dynamic> toJson() => {
        if (maxItemCount != null) 'max_item_count': maxItemCount,
        if (defaultItemCount != null) 'default_item_count': defaultItemCount,
        if (itemRefTypes != null) 'item_ref_types': itemRefTypes,
        if (defaultItemRefType != null) 'default_item_ref_type': defaultItemRefType,
        if (allowNestedComparisons != null) 'allow_nested_comparisons': allowNestedComparisons,
        if (allowedNestedTypes != null) 'allowed_nested_types': allowedNestedTypes,
      };
}

class ComparisonType {
  final String id;
  final String name;
  final String slug;
  final String? description;
  final String? icon;
  final String itemLabel;
  final int? itemCount;
  final int? fieldCount;
  final ComparisonTypeConfig? config;

  const ComparisonType({
    required this.id,
    required this.name,
    required this.slug,
    this.description,
    this.icon,
    required this.itemLabel,
    this.itemCount,
    this.fieldCount,
    this.config,
  });

  factory ComparisonType.fromJson(Map<String, dynamic> json) {
    return ComparisonType(
      id: json['id'] as String,
      name: json['name'] as String,
      slug: json['slug'] as String,
      description: json['description'] as String?,
      icon: json['icon'] as String?,
      itemLabel: json['item_label'] as String,
      itemCount: _parseInt(json['item_count']),
      fieldCount: _parseInt(json['field_count']),
      config: json['config'] == null
          ? null
          : ComparisonTypeConfig.fromJson(json['config'] as Map<String, dynamic>),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'slug': slug,
        if (description != null) 'description': description,
        if (icon != null) 'icon': icon,
        'item_label': itemLabel,
        if (itemCount != null) 'item_count': itemCount,
        if (fieldCount != null) 'field_count': fieldCount,
        if (config != null) 'config': config!.toJson(),
      };
}

class ResolvedItem {
  final String type;
  final String id;
  final String title;
  final String? description;
  final String? imageUrl;
  final String? url;
  final Map<String, dynamic>? metadata;

  const ResolvedItem({
    required this.type,
    required this.id,
    required this.title,
    this.description,
    this.imageUrl,
    this.url,
    this.metadata,
  });

  factory ResolvedItem.fromJson(Map<String, dynamic> json) {
    return ResolvedItem(
      type: json['type'] as String,
      id: json['id'] as String,
      title: json['title'] as String,
      description: json['description'] as String?,
      imageUrl: json['image_url'] as String?,
      url: json['url'] as String?,
      metadata: _parseMap(json['metadata']),
    );
  }

  factory ResolvedItem.fromJceItem(JceItem item) {
    final ref = item.itemRef ?? {};
    return ResolvedItem(
      type: (ref['type'] as String?) ?? '',
      id: (ref['id'] as String?) ?? (item.id ?? ''),
      title: item.title,
      description: item.description ?? (ref['description'] as String?),
      imageUrl: ref['image_url'] as String?,
      url: ref['url'] as String?,
      metadata: ref,
    );
  }

  Map<String, dynamic> toJson() => {
        'type': type,
        'id': id,
        'title': title,
        if (description != null) 'description': description,
        if (imageUrl != null) 'image_url': imageUrl,
        if (url != null) 'url': url,
        if (metadata != null) 'metadata': metadata,
      };
}

class JceItem {
  final String? id;
  final String title;
  final String? description;
  final String? typeId;
  final int? position;
  final Map<String, dynamic>? itemRef;

  const JceItem({
    this.id,
    required this.title,
    this.description,
    this.typeId,
    this.position,
    this.itemRef,
  });

  factory JceItem.fromJson(Map<String, dynamic> json) {
    return JceItem(
      id: json['id'] as String?,
      title: json['title'] as String,
      description: json['description'] as String?,
      typeId: json['type_id'] as String?,
      position: _parseInt(json['position']),
      itemRef: _parseMap(json['item_ref']),
    );
  }

  Map<String, dynamic> toJson() => {
        if (id != null) 'id': id,
        'title': title,
        if (description != null) 'description': description,
        if (typeId != null) 'type_id': typeId,
        if (position != null) 'position': position,
        if (itemRef != null) 'item_ref': itemRef,
      };

  ResolvedItem resolve() => ResolvedItem.fromJceItem(this);
}

class JceDimension {
  final String id;
  final String? contentId;
  final String label;
  final String? dimensionType;
  final int? position;

  const JceDimension({
    required this.id,
    this.contentId,
    required this.label,
    this.dimensionType,
    this.position,
  });

  factory JceDimension.fromJson(Map<String, dynamic> json) {
    return JceDimension(
      id: json['id'] as String,
      contentId: json['content_id'] as String?,
      label: json['label'] as String,
      dimensionType: json['dimension_type'] as String?,
      position: _parseInt(json['position']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        if (contentId != null) 'content_id': contentId,
        'label': label,
        if (dimensionType != null) 'dimension_type': dimensionType,
        if (position != null) 'position': position,
      };
}

class JceComparisonContent {
  final String id;
  final String title;
  final String slug;
  final String? body;
  final String? bodyHtml;
  final String? spaceId;
  final String? contentType;
  final String status;
  final String visibility;
  final String? authorId;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  const JceComparisonContent({
    required this.id,
    required this.title,
    required this.slug,
    this.body,
    this.bodyHtml,
    this.spaceId,
    this.contentType,
    required this.status,
    required this.visibility,
    this.authorId,
    this.createdAt,
    this.updatedAt,
  });

  factory JceComparisonContent.fromJson(Map<String, dynamic> json) {
    return JceComparisonContent(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      body: json['body'] as String?,
      bodyHtml: json['body_html'] as String?,
      spaceId: json['space_id'] as String?,
      contentType: json['content_type'] as String?,
      status: json['status'] as String,
      visibility: json['visibility'] as String,
      authorId: json['author_id'] as String?,
      createdAt: _parseDateTime(json['created_at']),
      updatedAt: _parseDateTime(json['updated_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (body != null) 'body': body,
        if (bodyHtml != null) 'body_html': bodyHtml,
        if (spaceId != null) 'space_id': spaceId,
        if (contentType != null) 'content_type': contentType,
        'status': status,
        'visibility': visibility,
        if (authorId != null) 'author_id': authorId,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

class JceComparisonMetadata {
  final String contentId;
  final String depth;
  final String? promotedFromId;
  final DateTime? promotedAt;
  final Map<String, dynamic>? config;

  const JceComparisonMetadata({
    required this.contentId,
    required this.depth,
    this.promotedFromId,
    this.promotedAt,
    this.config,
  });

  factory JceComparisonMetadata.fromJson(Map<String, dynamic> json) {
    return JceComparisonMetadata(
      contentId: json['content_id'] as String,
      depth: json['depth'] as String,
      promotedFromId: json['promoted_from_id'] as String?,
      promotedAt: _parseDateTime(json['promoted_at']),
      config: _parseMap(json['config']),
    );
  }

  Map<String, dynamic> toJson() => {
        'content_id': contentId,
        'depth': depth,
        if (promotedFromId != null) 'promoted_from_id': promotedFromId,
        if (promotedAt != null) 'promoted_at': promotedAt!.toIso8601String(),
        if (config != null) 'config': config,
      };
}

class JceComparisonDetail {
  final JceComparisonContent content;
  final JceComparisonMetadata metadata;
  final List<JceItem> items;
  final List<JceDimension> dimensions;
  final String? typeSlug;

  const JceComparisonDetail({
    required this.content,
    required this.metadata,
    required this.items,
    required this.dimensions,
    this.typeSlug,
  });

  factory JceComparisonDetail.fromJson(Map<String, dynamic> json) {
    return JceComparisonDetail(
      content: JceComparisonContent.fromJson(json['content'] as Map<String, dynamic>),
      metadata: JceComparisonMetadata.fromJson(json['metadata'] as Map<String, dynamic>),
      items: _parseObjectList(
        json['items'],
        (e) => JceItem.fromJson(e as Map<String, dynamic>),
      ),
      dimensions: _parseObjectList(
        json['dimensions'],
        (e) => JceDimension.fromJson(e as Map<String, dynamic>),
      ),
      typeSlug: json['type_slug'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'content': content.toJson(),
        'metadata': metadata.toJson(),
        'items': items.map((e) => e.toJson()).toList(),
        'dimensions': dimensions.map((e) => e.toJson()).toList(),
        if (typeSlug != null) 'type_slug': typeSlug,
      };

  List<ResolvedItem> get resolvedItems =>
      items.map((it) => it.resolve()).toList();
}

class JceComparisonSummary {
  final String id;
  final String title;
  final String slug;
  final String? body;
  final String? bodyHtml;
  final String? excerpt;
  final String status;
  final String visibility;
  final String depth;
  final String? authorId;
  final String? spaceId;
  final String? typeId;
  final String? typeSlug;
  final String? typeName;
  final int? viewCount;
  final int? commentCount;
  final int? shareCount;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  const JceComparisonSummary({
    required this.id,
    required this.title,
    required this.slug,
    this.body,
    this.bodyHtml,
    this.excerpt,
    required this.status,
    required this.visibility,
    required this.depth,
    this.authorId,
    this.spaceId,
    this.typeId,
    this.typeSlug,
    this.typeName,
    this.viewCount,
    this.commentCount,
    this.shareCount,
    this.createdAt,
    this.updatedAt,
  });

  factory JceComparisonSummary.fromJson(Map<String, dynamic> json) {
    return JceComparisonSummary(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      body: json['body'] as String?,
      bodyHtml: json['body_html'] as String?,
      excerpt: json['excerpt'] as String?,
      status: json['status'] as String,
      visibility: json['visibility'] as String,
      depth: json['depth'] as String,
      authorId: json['author_id'] as String?,
      spaceId: json['space_id'] as String?,
      typeId: json['type_id'] as String?,
      typeSlug: json['type_slug'] as String?,
      typeName: json['type_name'] as String?,
      viewCount: _parseInt(json['view_count']),
      commentCount: _parseInt(json['comment_count']),
      shareCount: _parseInt(json['share_count']),
      createdAt: _parseDateTime(json['created_at']),
      updatedAt: _parseDateTime(json['updated_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (body != null) 'body': body,
        if (bodyHtml != null) 'body_html': bodyHtml,
        if (excerpt != null) 'excerpt': excerpt,
        'status': status,
        'visibility': visibility,
        'depth': depth,
        if (authorId != null) 'author_id': authorId,
        if (spaceId != null) 'space_id': spaceId,
        if (typeId != null) 'type_id': typeId,
        if (typeSlug != null) 'type_slug': typeSlug,
        if (typeName != null) 'type_name': typeName,
        if (viewCount != null) 'view_count': viewCount,
        if (commentCount != null) 'comment_count': commentCount,
        if (shareCount != null) 'share_count': shareCount,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };

  Map<String, dynamic> toCardMap() => {
        'title': title,
        'subtitle': excerpt ??
            '${typeName ?? ''} · $depth'.trim().replaceAll(RegExp(r'^·\s*'), ''),
        'avatar_url': null,
      };
}

class MatrixItem {
  final String id;
  final String title;
  final String? description;
  final String? imageUrl;
  final int? position;
  final List<MatrixDimension> dimensions;

  const MatrixItem({
    required this.id,
    required this.title,
    this.description,
    this.imageUrl,
    this.position,
    required this.dimensions,
  });

  factory MatrixItem.fromJson(Map<String, dynamic> json) {
    return MatrixItem(
      id: json['id'] as String,
      title: json['title'] as String,
      description: json['description'] as String?,
      imageUrl: json['image_url'] as String?,
      position: _parseInt(json['position']),
      dimensions: _parseObjectList(
        json['dimensions'],
        (e) => MatrixDimension.fromJson(e as Map<String, dynamic>),
      ),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        if (description != null) 'description': description,
        if (imageUrl != null) 'image_url': imageUrl,
        if (position != null) 'position': position,
        'dimensions': dimensions.map((e) => e.toJson()).toList(),
      };
}

class MatrixDimension {
  final String dimensionId;
  final String label;
  final String type;
  final String? value;
  final double? valueRating;
  final bool? valueBoolean;

  const MatrixDimension({
    required this.dimensionId,
    required this.label,
    required this.type,
    this.value,
    this.valueRating,
    this.valueBoolean,
  });

  factory MatrixDimension.fromJson(Map<String, dynamic> json) {
    return MatrixDimension(
      dimensionId: json['dimension_id'] as String,
      label: json['label'] as String,
      type: json['type'] as String,
      value: json['value'] as String?,
      valueRating: _parseDouble(json['value_rating']),
      valueBoolean: json['value_boolean'] as bool?,
    );
  }

  Map<String, dynamic> toJson() => {
        'dimension_id': dimensionId,
        'label': label,
        'type': type,
        if (value != null) 'value': value,
        if (valueRating != null) 'value_rating': valueRating,
        if (valueBoolean != null) 'value_boolean': valueBoolean,
      };
}

class SelectionToken {
  final String token;

  const SelectionToken({required this.token});

  factory SelectionToken.fromJson(Map<String, dynamic> json) {
    return SelectionToken(token: json['token'] as String);
  }

  Map<String, dynamic> toJson() => {'token': token};
}

class CreateQuickComparisonInput {
  final String title;
  final String? spaceId;
  final String typeId;
  final String depth;
  final String status;
  final String visibility;
  final List<JceItem> items;
  final List<String> dimensions;
  final Map<String, dynamic>? config;

  const CreateQuickComparisonInput({
    required this.title,
    this.spaceId,
    required this.typeId,
    this.depth = 'quick',
    this.status = 'draft',
    this.visibility = 'private',
    required this.items,
    required this.dimensions,
    this.config,
  });

  Map<String, dynamic> toJson() => {
        'title': title,
        if (spaceId != null) 'space_id': spaceId,
        'type_id': typeId,
        'depth': depth,
        'status': status,
        'visibility': visibility,
        'items': items.map((e) => e.toJson()).toList(),
        'dimensions': dimensions,
        if (config != null) 'config': config,
      };
}

class DataSource {
  final String id;

  final String? entityType;
  final String? entityId;
  final String? dimensionSlug;
  final String sourceName;
  final String? sourceUrl;
  final String sourceKind;
  final double trustLevel;
  final double freshnessScore;
  final DateTime? lastVerifiedAt;
  final Map<String, dynamic>? config;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  const DataSource({
    required this.id,

    this.entityType,
    this.entityId,
    this.dimensionSlug,
    required this.sourceName,
    this.sourceUrl,
    required this.sourceKind,
    required this.trustLevel,
    required this.freshnessScore,
    this.lastVerifiedAt,
    this.config,
    this.createdAt,
    this.updatedAt,
  });

  factory DataSource.fromJson(Map<String, dynamic> json) {
    return DataSource(
      id: json['id'] as String,

      entityType: json['entity_type'] as String?,
      entityId: json['entity_id'] as String?,
      dimensionSlug: json['dimension_slug'] as String?,
      sourceName: json['source_name'] as String,
      sourceUrl: json['source_url'] as String?,
      sourceKind: json['source_kind'] as String,
      trustLevel: _parseDouble(json['trust_level']) ?? 0,
      freshnessScore: _parseDouble(json['freshness_score']) ?? 0,
      lastVerifiedAt: _parseDateTime(json['last_verified_at']),
      config: _parseMap(json['config']),
      createdAt: _parseDateTime(json['created_at']),
      updatedAt: _parseDateTime(json['updated_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,

        if (entityType != null) 'entity_type': entityType,
        if (entityId != null) 'entity_id': entityId,
        if (dimensionSlug != null) 'dimension_slug': dimensionSlug,
        'source_name': sourceName,
        if (sourceUrl != null) 'source_url': sourceUrl,
        'source_kind': sourceKind,
        'trust_level': trustLevel,
        'freshness_score': freshnessScore,
        if (lastVerifiedAt != null) 'last_verified_at': lastVerifiedAt!.toIso8601String(),
        if (config != null) 'config': config,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

class FreshnessCell {
  final String dimensionId;
  final String label;
  final double freshnessScore;
  final String? sourceName;
  final String? sourceKind;
  final DateTime? retrievedAt;
  final bool isStale;

  const FreshnessCell({
    required this.dimensionId,
    required this.label,
    required this.freshnessScore,
    this.sourceName,
    this.sourceKind,
    this.retrievedAt,
    required this.isStale,
  });

  factory FreshnessCell.fromJson(Map<String, dynamic> json) {
    return FreshnessCell(
      dimensionId: json['dimension_id'] as String,
      label: json['label'] as String,
      freshnessScore: _parseDouble(json['freshness_score']) ?? 0,
      sourceName: json['source_name'] as String?,
      sourceKind: json['source_kind'] as String?,
      retrievedAt: _parseDateTime(json['retrieved_at']),
      isStale: json['is_stale'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() => {
        'dimension_id': dimensionId,
        'label': label,
        'freshness_score': freshnessScore,
        if (sourceName != null) 'source_name': sourceName,
        if (sourceKind != null) 'source_kind': sourceKind,
        if (retrievedAt != null) 'retrieved_at': retrievedAt!.toIso8601String(),
        'is_stale': isStale,
      };
}

class FreshnessItem {
  final String itemId;
  final String itemTitle;
  final double freshnessScore;
  final List<FreshnessCell> cells;

  const FreshnessItem({
    required this.itemId,
    required this.itemTitle,
    required this.freshnessScore,
    required this.cells,
  });

  factory FreshnessItem.fromJson(Map<String, dynamic> json) {
    return FreshnessItem(
      itemId: json['item_id'] as String,
      itemTitle: json['item_title'] as String,
      freshnessScore: _parseDouble(json['freshness_score']) ?? 0,
      cells: _parseObjectList(
        json['cells'],
        (e) => FreshnessCell.fromJson(e as Map<String, dynamic>),
      ),
    );
  }

  Map<String, dynamic> toJson() => {
        'item_id': itemId,
        'item_title': itemTitle,
        'freshness_score': freshnessScore,
        'cells': cells.map((e) => e.toJson()).toList(),
      };
}

class FreshnessSummary {
  final String contentId;
  final double overallFreshnessScore;
  final int staleCount;
  final int totalCount;
  final List<FreshnessItem> byItem;

  const FreshnessSummary({
    required this.contentId,
    required this.overallFreshnessScore,
    required this.staleCount,
    required this.totalCount,
    required this.byItem,
  });

  factory FreshnessSummary.fromJson(Map<String, dynamic> json) {
    return FreshnessSummary(
      contentId: json['content_id'] as String,
      overallFreshnessScore: _parseDouble(json['overall_freshness_score']) ?? 0,
      staleCount: _parseInt(json['stale_count']) ?? 0,
      totalCount: _parseInt(json['total_count']) ?? 0,
      byItem: _parseObjectList(
        json['by_item'],
        (e) => FreshnessItem.fromJson(e as Map<String, dynamic>),
      ),
    );
  }

  Map<String, dynamic> toJson() => {
        'content_id': contentId,
        'overall_freshness_score': overallFreshnessScore,
        'stale_count': staleCount,
        'total_count': totalCount,
        'by_item': byItem.map((e) => e.toJson()).toList(),
      };
}

class IngestionJob {
  final String id;

  final String contentId;
  final String status;
  final String? sourceKind;
  final int refreshedCount;
  final String? error;
  final DateTime? startedAt;
  final DateTime? completedAt;
  final DateTime createdAt;
  final DateTime? updatedAt;

  const IngestionJob({
    required this.id,

    required this.contentId,
    required this.status,
    this.sourceKind,
    required this.refreshedCount,
    this.error,
    this.startedAt,
    this.completedAt,
    required this.createdAt,
    this.updatedAt,
  });

  factory IngestionJob.fromJson(Map<String, dynamic> json) {
    return IngestionJob(
      id: json['id'] as String,

      contentId: json['content_id'] as String,
      status: json['status'] as String,
      sourceKind: json['source_kind'] as String?,
      refreshedCount: _parseInt(json['refreshed_count']) ?? 0,
      error: json['error'] as String?,
      startedAt: _parseDateTime(json['started_at']),
      completedAt: _parseDateTime(json['completed_at']),
      createdAt: _parseDateTime(json['created_at']) ?? DateTime.now(),
      updatedAt: _parseDateTime(json['updated_at']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,

        'content_id': contentId,
        'status': status,
        if (sourceKind != null) 'source_kind': sourceKind,
        'refreshed_count': refreshedCount,
        if (error != null) 'error': error,
        if (startedAt != null) 'started_at': startedAt!.toIso8601String(),
        if (completedAt != null) 'completed_at': completedAt!.toIso8601String(),
        'created_at': createdAt.toIso8601String(),
        if (updatedAt != null) 'updated_at': updatedAt!.toIso8601String(),
      };
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

DateTime? _parseDateTime(dynamic value) {
  if (value == null) return null;
  if (value is DateTime) return value;
  if (value is String) return DateTime.tryParse(value);
  return null;
}

int? _parseInt(dynamic value) {
  if (value == null) return null;
  if (value is int) return value;
  if (value is double) return value.toInt();
  if (value is num) return value.toInt();
  if (value is String) return int.tryParse(value);
  return null;
}

double? _parseDouble(dynamic value) {
  if (value == null) return null;
  if (value is double) return value;
  if (value is int) return value.toDouble();
  if (value is num) return value.toDouble();
  if (value is String) return double.tryParse(value);
  return null;
}

Map<String, dynamic>? _parseMap(dynamic value) {
  if (value == null) return null;
  if (value is Map) {
    return value.map((k, v) => MapEntry(k.toString(), v));
  }
  return null;
}

List<String>? _parseStringList(dynamic value) {
  if (value is! List) return null;
  return value.whereType<String>().toList();
}

List<int>? _parseIntList(dynamic value) {
  if (value is! List) return null;
  return value.map(_parseInt).whereType<int>().toList();
}

List<T> _parseObjectList<T>(
  dynamic value,
  T Function(dynamic) parser,
) {
  if (value is! List) return [];
  return value.map(parser).whereType<T>().toList();
}
