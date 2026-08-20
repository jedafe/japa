class Job {
  final String id;
  final String title;
  final String slug;
  final String? summary;
  final String? body;
  final Map<String, String>? fieldValues;
  final DateTime? publishedAt;

  Job({
    required this.id,
    required this.title,
    required this.slug,
    this.summary,
    this.body,
    this.fieldValues,
    this.publishedAt,
  });

  factory Job.fromJson(Map<String, dynamic> json) {
    final fv = json['field_values'];
    return Job(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      summary: json['summary'] as String?,
      body: json['body'] as String?,
      fieldValues: fv is Map<String, dynamic>
          ? fv.map((k, v) => MapEntry(k, v.toString()))
          : null,
      publishedAt: _parseDateTime(json['published_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  /// Detail screens expect a `description` field; prefer the full body,
  /// falling back to the summary.
  String? get description => body ?? summary;

  /// Map representation for generic card widgets.
  Map<String, dynamic> toCardMap() => {
        'title': title,
        'subtitle': summary ?? '',
        'avatar_url': null,
      };

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (summary != null) 'summary': summary,
        if (body != null) 'body': body,
        if (fieldValues != null) 'field_values': fieldValues,
        if (publishedAt != null) 'published_at': publishedAt!.toIso8601String(),
      };
}
