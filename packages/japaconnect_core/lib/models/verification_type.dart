/// A managed verification badge type from GET /verification-requests/types
/// (JEOS §07 — 001013 badge taxonomy). `badgeUrl` carries the admin-uploaded
/// artwork; when null, surfaces should fall back to the generic verified icon.
class VerificationType {
  final String value;
  final String label;
  final String scope;
  final String? badgeUrl;

  const VerificationType({
    required this.value,
    required this.label,
    required this.scope,
    this.badgeUrl,
  });

  factory VerificationType.fromJson(Map<String, dynamic> json) {
    return VerificationType(
      value: json['value'] as String? ?? '',
      label: json['label'] as String? ?? '',
      scope: json['scope'] as String? ?? 'user',
      badgeUrl: json['badge_url'] as String?,
    );
  }
}
