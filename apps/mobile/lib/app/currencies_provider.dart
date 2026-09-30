import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

/// Loads the active managed currency table once per app session so any
/// screen can resolve a currency code to its name/symbol.
/// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
final currenciesProvider = FutureProvider<List<Currency>>((ref) async {
  final client = ref.watch(jcApiClientProvider);
  return client.get<List<Currency>>(
    '/currencies',
    parse: (json) {
      final data = (json as Map<String, dynamic>)['data'] as List<dynamic>? ?? [];
      return data
          .cast<Map<String, dynamic>>()
          .map(Currency.fromJson)
          .where((c) => c.code.isNotEmpty)
          .toList();
    },
  );
});
