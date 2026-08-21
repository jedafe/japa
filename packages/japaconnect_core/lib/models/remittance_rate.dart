class RemittanceRate {
  final String id;
  final String providerId;
  final String? providerName;
  final String? providerSlug;
  final String? providerWebsite;
  final String sendCurrency;
  final String receiveCurrency;
  final double rate;
  final double? feeFlat;
  final double? feePct;
  final String? transferTime;
  final DateTime? fetchedAt;
  final bool? isManual;
  final String? sourceUrl;

  RemittanceRate({
    required this.id,
    required this.providerId,
    this.providerName,
    this.providerSlug,
    this.providerWebsite,
    required this.sendCurrency,
    required this.receiveCurrency,
    required this.rate,
    this.feeFlat,
    this.feePct,
    this.transferTime,
    this.fetchedAt,
    this.isManual,
    this.sourceUrl,
  });

  factory RemittanceRate.fromJson(Map<String, dynamic> json) {
    return RemittanceRate(
      id: json['id'] as String,
      providerId: json['provider_id'] as String,
      providerName: json['provider_name'] as String?,
      providerSlug: json['provider_slug'] as String?,
      providerWebsite: json['provider_website'] as String?,
      sendCurrency: json['send_currency'] as String,
      receiveCurrency: json['receive_currency'] as String,
      rate: (json['rate'] as num).toDouble(),
      feeFlat: (json['fee_flat'] as num?)?.toDouble(),
      feePct: (json['fee_pct'] as num?)?.toDouble(),
      transferTime: json['transfer_time'] as String?,
      fetchedAt: _parseDateTime(json['fetched_at']),
      isManual: json['is_manual'] as bool?,
      sourceUrl: json['source_url'] as String?,
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
        'provider_id': providerId,
        if (providerName != null) 'provider_name': providerName,
        if (providerSlug != null) 'provider_slug': providerSlug,
        if (providerWebsite != null) 'provider_website': providerWebsite,
        'send_currency': sendCurrency,
        'receive_currency': receiveCurrency,
        'rate': rate,
        if (feeFlat != null) 'fee_flat': feeFlat,
        if (feePct != null) 'fee_pct': feePct,
        if (transferTime != null) 'transfer_time': transferTime,
        if (fetchedAt != null) 'fetched_at': fetchedAt!.toIso8601String(),
        if (isManual != null) 'is_manual': isManual,
        if (sourceUrl != null) 'source_url': sourceUrl,
      };
}
