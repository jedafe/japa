class RemittanceProvider {
  final String id;
  final String name;
  final String? rate;

  RemittanceProvider({
    required this.id,
    required this.name,
    this.rate,
  });

  factory RemittanceProvider.fromJson(Map<String, dynamic> json) {
    return RemittanceProvider(
      id: json['id'] as String,
      name: json['name'] as String,
      rate: json['rate']?.toString(),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        if (rate != null) 'rate': rate,
      };
}
