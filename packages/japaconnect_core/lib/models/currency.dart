/// Managed credit currency (Section 58, ADR-065).
/// Returned by GET /api/v1/currencies (public) and /api/v1/wallet/currencies.
class Currency {
  final String code;
  final String name;
  final String symbol;
  final int creditsPerUnit;
  final bool isActive;
  final int position;

  const Currency({
    required this.code,
    required this.name,
    required this.symbol,
    this.creditsPerUnit = 0,
    this.isActive = true,
    this.position = 0,
  });

  factory Currency.fromJson(Map<String, dynamic> json) {
    return Currency(
      code: json['code'] as String? ?? '',
      name: json['name'] as String? ?? '',
      symbol: json['symbol'] as String? ?? '',
      creditsPerUnit: (json['credits_per_unit'] as num?)?.toInt() ?? 0,
      isActive: json['is_active'] as bool? ?? true,
      position: (json['position'] as num?)?.toInt() ?? 0,
    );
  }

  /// Display label for selectors, e.g. "USD — US Dollar".
  String get label => name.isEmpty ? code : '$code — $name';
}
