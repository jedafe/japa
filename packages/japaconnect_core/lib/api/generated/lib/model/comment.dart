//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class Comment {
  /// Returns a new [Comment] instance.
  Comment({
    this.authorId,
    this.body,
    this.contentId,
    this.createdAt,
    this.id,
    this.parentId,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? authorId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? body;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? contentId;

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
  String? id;

  /// Parent comment ID for nested replies
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? parentId;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Comment &&
          other.authorId == authorId &&
          other.body == body &&
          other.contentId == contentId &&
          other.createdAt == createdAt &&
          other.id == id &&
          other.parentId == parentId;

  @override
  int get hashCode =>
      // ignore: unnecessary_parenthesis
      (authorId == null ? 0 : authorId!.hashCode) +
      (body == null ? 0 : body!.hashCode) +
      (contentId == null ? 0 : contentId!.hashCode) +
      (createdAt == null ? 0 : createdAt!.hashCode) +
      (id == null ? 0 : id!.hashCode) +
      (parentId == null ? 0 : parentId!.hashCode);

  @override
  String toString() =>
      'Comment[authorId=$authorId, body=$body, contentId=$contentId, createdAt=$createdAt, id=$id, parentId=$parentId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.authorId != null) {
      json[r'author_id'] = this.authorId;
    } else {
      json[r'author_id'] = null;
    }
    if (this.body != null) {
      json[r'body'] = this.body;
    } else {
      json[r'body'] = null;
    }
    if (this.contentId != null) {
      json[r'content_id'] = this.contentId;
    } else {
      json[r'content_id'] = null;
    }
    if (this.createdAt != null) {
      json[r'created_at'] = this.createdAt!.toUtc().toIso8601String();
    } else {
      json[r'created_at'] = null;
    }
    if (this.id != null) {
      json[r'id'] = this.id;
    } else {
      json[r'id'] = null;
    }
    if (this.parentId != null) {
      json[r'parent_id'] = this.parentId;
    } else {
      json[r'parent_id'] = null;
    }
    return json;
  }

  /// Returns a new [Comment] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static Comment? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        requiredKeys.forEach((key) {
          assert(json.containsKey(key),
              'Required key "Comment[$key]" is missing from JSON.');
          assert(json[key] != null,
              'Required key "Comment[$key]" has a null value in JSON.');
        });
        return true;
      }());

      return Comment(
        authorId: mapValueOfType<String>(json, r'author_id'),
        body: mapValueOfType<String>(json, r'body'),
        contentId: mapValueOfType<String>(json, r'content_id'),
        createdAt: mapDateTime(json, r'created_at', r''),
        id: mapValueOfType<String>(json, r'id'),
        parentId: mapValueOfType<String>(json, r'parent_id'),
      );
    }
    return null;
  }

  static List<Comment> listFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final result = <Comment>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = Comment.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, Comment> mapFromJson(dynamic json) {
    final map = <String, Comment>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = Comment.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of Comment-objects as value to a dart map
  static Map<String, List<Comment>> mapListFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final map = <String, List<Comment>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = Comment.listFromJson(
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
