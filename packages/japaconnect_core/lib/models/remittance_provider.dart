class RemittanceProvider {
  final String id;
  final String name;
  final String? slug;
  final String? websiteUrl;
  final String? dataSource;
  final bool isActive;
  final bool isBenchmark;
  final int? displayOrder;
  final DateTime? createdAt;

  RemittanceProvider({
    required this.id,
    required this.name,
    this.slug,
    this.websiteUrl,
    this.dataSource,
    this.isActive = true,
    this.isBenchmark = false,
    this.displayOrder,
    this.createdAt,
  });

  factory RemittanceProvider.fromJson(Map<String, dynamic> json) {
    return RemittanceProvider(
      id: json['id'] as String,
      name: json['name'] as String,
      slug: json['slug'] as String?,
      websiteUrl: json['website_url'] as String?,
      dataSource: json['data_source'] as String?,
      isActive: json['is_active'] as bool? ?? true,
      isBenchmark: json['is_benchmark'] as bool? ?? false,
      displayOrder: json['display_order'] as int?,
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
        'name': name,
        if (slug != null) 'slug': slug,
        if (websiteUrl != null) 'website_url': websiteUrl,
        if (dataSource != null) 'data_source': dataSource,
        'is_active': isActive,
        'is_benchmark': isBenchmark,
        if (displayOrder != null) 'display_order': displayOrder,
        if (createdAt != null) 'created_at': createdAt!.toIso8601String(),
      };
}
