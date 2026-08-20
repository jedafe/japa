//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class PublicTrendingContentGet200Response {
  /// Returns a new [PublicTrendingContentGet200Response] instance.
  PublicTrendingContentGet200Response({
    this.computedAt,
    this.data = const [],
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  DateTime? computedAt;

  List<TrendingItem> data;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is PublicTrendingContentGet200Response &&
          other.computedAt == computedAt &&
          _deepEquality.equals(other.data, data);

  @override
  int get hashCode =>
      // ignore: unnecessary_parenthesis
      (computedAt == null ? 0 : computedAt!.hashCode) + (data.hashCode);

  @override
  String toString() =>
      'PublicTrendingContentGet200Response[computedAt=$computedAt, data=$data]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.computedAt != null) {
      json[r'computed_at'] = this.computedAt!.toUtc().toIso8601String();
    } else {
      json[r'computed_at'] = null;
    }
    json[r'data'] = this.data;
    return json;
  }

  /// Returns a new [PublicTrendingContentGet200Response] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static PublicTrendingContentGet200Response? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        requiredKeys.forEach((key) {
          assert(json.containsKey(key),
              'Required key "PublicTrendingContentGet200Response[$key]" is missing from JSON.');
          assert(json[key] != null,
              'Required key "PublicTrendingContentGet200Response[$key]" has a null value in JSON.');
        });
        return true;
      }());

      return PublicTrendingContentGet200Response(
        computedAt: mapDateTime(json, r'computed_at', r''),
        data: TrendingItem.listFromJson(json[r'data']),
      );
    }
    return null;
  }

  static List<PublicTrendingContentGet200Response> listFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final result = <PublicTrendingContentGet200Response>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = PublicTrendingContentGet200Response.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, PublicTrendingContentGet200Response> mapFromJson(
      dynamic json) {
    final map = <String, PublicTrendingContentGet200Response>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = PublicTrendingContentGet200Response.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of PublicTrendingContentGet200Response-objects as value to a dart map
  static Map<String, List<PublicTrendingContentGet200Response>> mapListFromJson(
    dynamic json, {
    bool growable = false,
  }) {
    final map = <String, List<PublicTrendingContentGet200Response>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = PublicTrendingContentGet200Response.listFromJson(
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
