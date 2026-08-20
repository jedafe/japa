/// Canonical geo space model returned by the public geo hierarchy endpoints
/// (`/geo/countries`, `/geo/countries/{code}/states`, `/geo/states/{id}/cities`).
///
/// Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
class GeoSpace {
  final String id;
  final String name;
  final String slug;
  final String level;
  final String? geoCode;
  final String? description;
  final String? currencyCode;
  final int memberCount;
  final int postCount;
  final int followerCount;

  GeoSpace({
    required this.id,
    required this.name,
    required this.slug,
    required this.level,
    this.geoCode,
    this.description,
    this.currencyCode,
    this.memberCount = 0,
    this.postCount = 0,
    this.followerCount = 0,
  });

  factory GeoSpace.fromJson(Map<String, dynamic> json) {
    return GeoSpace(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      slug: json['slug'] as String? ?? '',
      level: json['level'] as String? ?? '',
      geoCode: json['geo_code'] as String?,
      description: json['description'] as String?,
      currencyCode: json['currency_code'] as String?,
      memberCount: (json['member_count'] as num?)?.toInt() ?? 0,
      postCount: (json['post_count'] as num?)?.toInt() ?? 0,
      followerCount: (json['follower_count'] as num?)?.toInt() ?? 0,
    );
  }

  bool get isCountry => level == 'country';
  bool get isState => level == 'state';
  bool get isCity => level == 'city';
}
