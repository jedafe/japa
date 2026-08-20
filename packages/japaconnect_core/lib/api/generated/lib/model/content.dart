//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class Content {
  /// Returns a new [Content] instance.
  Content({
    this.authorId,
    this.contentType,
    this.createdAt,
    this.excerpt,
    this.id,
    this.readTime,
    this.slug,
    this.spaceId,
    this.tags = const [],
    this.title,
    this.updatedAt,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? authorId;

  ContentContentTypeEnum? contentType;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  DateTime? createdAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? excerpt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? id;

  /// Estimated read time in minutes
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? readTime;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? slug;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? spaceId;

  List<String> tags;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  DateTime? updatedAt;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Content &&
          other.authorId == authorId &&
          other.contentType == contentType &&
          other.createdAt == createdAt &&
          other.excerpt == excerpt &&
          other.id == id &&
          other.readTime == readTime &&
          other.slug == slug &&
          other.spaceId == spaceId &&
          _deepEquality.equals(other.tags, tags) &&
          other.title == title &&
          other.updatedAt == updatedAt;

  @override
  int get hashCode =>
      // ignore: unnecessary_parenthesis
      (authorId == null ? 0 : authorId!.hashCode) +
      (contentType == null ? 0 : contentType!.hashCode) +
      (createdAt == null ? 0 : createdAt!.hashCode) +
      (excerpt == null ? 0 : excerpt!.hashCode) +
      (id == null ? 0 : id!.hashCode) +
      (readTime == null ? 0 : readTime!.hashCode) +
      (slug == null ? 0 : slug!.hashCode) +
      (spaceId == null ? 0 : spaceId!.hashCode) +
      (tags.hashCode) +
      (title == null ? 0 : title!.hashCode) +
      (updatedAt == null ? 0 : updatedAt!.hashCode);

  @override
  String toString() =>
      'Content[authorId=$authorId, contentType=$contentType, createdAt=$createdAt, excerpt=$excerpt, id=$id, readTime=$readTime, slug=$slug, spaceId=$spaceId, tags=$tags, title=$title, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.authorId != null) {
      json[r'author_id'] = this.authorId;
    } else {
      json[r'author_id'] = null;
    }
    if (this.contentType != null) {
      json[r'content_type'] = this.contentType;
    } else {
      json[r'content_type'] = null;
    }
    if (this.createdAt != null) {
      json[r'created_at'] = this.createdAt!.toUtc().toIso8601String();
    } else {
      json[r'created_at'] = null;
    }
    if (this.excerpt != null) {
      json[r'excerpt'] = this.excerpt;
    } else {
      json[r'excerpt'] = null;
    }
    if (this.id != null) {
      json[r'id'] = this.id;
    } else {
      json[r'id'] = null;
    }
    if (this.readTime != null) {
      json[r'read_time'] = this.readTime;
    } else {
      json[r'read_time'] = null;
    }
    if (this.slug != null) {
      json[r'slug'] = this.slug;
    } else {
      json[r'slug'] = null;
    }
    if (this.spaceId != null) {
      json[r'space_id'] = this.spaceId;
    } else {
      json[r'space_id'] = null;
    }
    json[r'tags'] = this.tags;
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
    }
    if (this.updatedAt != null) {
      json[r'updated_at'] = this.updatedAt!.toUtc().toIso8601String();
    } else {
      json[r'updated_at'] = null;
    }
    return json;
  }

  /// Returns a new [Content] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static Content? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        requiredKeys.forEach((key) {
          assert(json.containsKey(key),
              'Required key "Content[$key]" is missing from JSON.');
          assert(json[key] != null,
              'Required key "Content[$key]" has a null value in JSON.');
        });
        return true;
      }());

      return Content(
        authorId: mapValueOfType<String>(json, r'author_id'),
        contentType: ContentContentTypeEnum.fromJson(json[r'content_type']),
        createdAt: mapDateTime(json, r'created_at', r''),
        excerpt: mapValueOfType<String>(json, r'excerpt'),
        id: mapValueOfType<String>(json, r'id'),
        readTime: mapValueOfType<int>(json, r'read_time'),
        slug: mapValueOfType<String>(json, r'slug'),
        spaceId: mapValueOfType<String>(json, r'space_id'),
        tags: json[r'tags'] is Iterable
            ? (json[r'tags'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        title: mapValueOfType<String>(json, r'title'),
        updatedAt: mapDateTime(json, r'updated_at', r''),
      );
    }
    return null;
  }

  static List<Content> listFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final result = <Content>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = Content.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, Content> mapFromJson(dynamic json) {
    final map = <String, Content>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = Content.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of Content-objects as value to a dart map
  static Map<String, List<Content>> mapListFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final map = <String, List<Content>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = Content.listFromJson(
          entry.value,
          growable: growable,
        );
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{};
}

class ContentContentTypeEnum {
  /// Instantiate a new enum with the provided [value].
  const ContentContentTypeEnum._(this.value);

  /// The underlying value of this enum member.
  final String value;

  @override
  String toString() => value;

  String toJson() => value;

  static const post = ContentContentTypeEnum._(r'post');
  static const article = ContentContentTypeEnum._(r'article');
  static const guide = ContentContentTypeEnum._(r'guide');

  /// List of all possible values in this [enum][ContentContentTypeEnum].
  static const values = <ContentContentTypeEnum>[
    post,
    article,
    guide,
  ];

  static ContentContentTypeEnum? fromJson(dynamic value) =>
      ContentContentTypeEnumTypeTransformer().decode(value);

  static List<ContentContentTypeEnum> listFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final result = <ContentContentTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = ContentContentTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [ContentContentTypeEnum] to String,
/// and [decode] dynamic data back to [ContentContentTypeEnum].
class ContentContentTypeEnumTypeTransformer {
  factory ContentContentTypeEnumTypeTransformer() =>
      _instance ??= const ContentContentTypeEnumTypeTransformer._();

  const ContentContentTypeEnumTypeTransformer._();

  String encode(ContentContentTypeEnum data) => data.value;

  /// Decodes a [dynamic value][data] to a ContentContentTypeEnum.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  ContentContentTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data != null) {
      switch (data) {
        case r'post':
          return ContentContentTypeEnum.post;
        case r'article':
          return ContentContentTypeEnum.article;
        case r'guide':
          return ContentContentTypeEnum.guide;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// Singleton [ContentContentTypeEnumTypeTransformer] instance.
  static ContentContentTypeEnumTypeTransformer? _instance;
}
