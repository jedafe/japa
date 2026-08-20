class Wallet {
  final String? currencyCode;
  final num? balance;
  final num? available;
  final num? held;

  Wallet({
    this.currencyCode,
    this.balance,
    this.available,
    this.held,
  });

  factory Wallet.fromJson(Map<String, dynamic> json) {
    return Wallet(
      currencyCode: json['currency_code'] as String? ?? json['currency'] as String?,
      balance: json['balance'] as num?,
      available: json['available'] as num?,
      held: json['held'] as num?,
    );
  }

  Map<String, dynamic> toJson() => {
        if (currencyCode != null) 'currency_code': currencyCode,
        if (balance != null) 'balance': balance,
        if (available != null) 'available': available,
        if (held != null) 'held': held,
      };
}
