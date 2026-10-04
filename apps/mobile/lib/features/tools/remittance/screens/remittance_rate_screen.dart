import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/app/currencies_provider.dart';
import 'package:japaconnect_mobile/features/tools/remittance/providers/remittance_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class RemittanceRateScreen extends ConsumerStatefulWidget {
  const RemittanceRateScreen({super.key});

  @override
  ConsumerState<RemittanceRateScreen> createState() => _RemittanceRateScreenState();
}

class _RemittanceRateScreenState extends ConsumerState<RemittanceRateScreen> {
  final _sendController = TextEditingController(text: 'GBP');
  final _receiveController = TextEditingController(text: 'NGN');
  String _send = 'GBP';
  String _receive = 'NGN';
  ({String send, String receive})? _search;

  @override
  void dispose() {
    _sendController.dispose();
    _receiveController.dispose();
    super.dispose();
  }

  void _doSearch() {
    setState(() {
      _search = (send: _send.trim().toUpperCase(),
          receive: _receive.trim().toUpperCase());
    });
  }

  /// Currency picker backed by the managed currency table (§58). Falls back
  /// to a free-text field when the currency list cannot be loaded so legacy
  /// or deactivated codes remain enterable.
  Widget _currencyPicker({
    required String label,
    required String value,
    required TextEditingController fallbackController,
    required ValueChanged<String> onChanged,
    required List<Currency>? currencies,
  }) {
    if (currencies == null) {
      return TextField(
        controller: fallbackController,
        decoration: InputDecoration(
          labelText: label,
          border: const OutlineInputBorder(),
        ),
        textCapitalization: TextCapitalization.characters,
        onChanged: (v) => onChanged(v),
        onSubmitted: (_) => _doSearch(),
      );
    }
    // Keep a stored/typed code selectable even when it isn't in the managed
    // list (e.g. a currency deactivated later) — the item is added below.
    final items = currencies.map((c) => c.code).toSet();
    final selected = items.contains(value) ? value : null;
    return DropdownButtonFormField<String>(
      key: ValueKey('${label}_$value'),
      initialValue: value.isNotEmpty ? value : null,
      decoration: InputDecoration(
        labelText: label,
        border: const OutlineInputBorder(),
      ),
      items: [
        for (final c in currencies)
          DropdownMenuItem(value: c.code, child: Text(c.label)),
        if (selected == null && value.isNotEmpty)
          DropdownMenuItem(value: value, child: Text(value)),
      ],
      onChanged: (v) {
        if (v != null) {
          onChanged(v);
          _doSearch();
        }
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final currencies = ref.watch(currenciesProvider).value;
    final rates = _search != null
        ? ref.watch(remittanceRatesProvider(_search!))
        : const AsyncData<List<RemittanceRate>>([]);
    final symbolByCode = {
      for (final c in currencies ?? const <Currency>[]) c.code.toUpperCase(): c.symbol,
    };
    // "£ GBP" — symbol (when managed) before the code.
    String currencyChip(String code) {
      final sym = symbolByCode[code.toUpperCase()];
      return sym != null && sym.isNotEmpty ? '$sym $code' : code;
    }

    // Adaptive precision: small rates need decimals, large FX rates don't.
    String formatRate(double rate) {
      if (rate >= 100) return rate.toStringAsFixed(2);
      if (rate >= 1) return rate.toStringAsFixed(4);
      return rate.toStringAsFixed(6);
    }

    // Sellable quotes sorted best-first; benchmark (mid-market) reference
    // rows pinned at the bottom — §74/ADR-075.
    List<RemittanceRate> sortedRates(List<RemittanceRate> items) {
      final sellable = items.where((r) => !r.isBenchmark).toList()
        ..sort((a, b) => b.rate.compareTo(a.rate));
      final benchmark = items.where((r) => r.isBenchmark).toList()
        ..sort((a, b) => b.rate.compareTo(a.rate));
      return [...sellable, ...benchmark];
    }

    return Scaffold(
      appBar: AppBar(title: Text(t('tools.remittance_rates'))),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                Expanded(
                  child: _currencyPicker(
                    label: t('tools.send_currency'),
                    value: _send,
                    fallbackController: _sendController,
                    currencies: currencies,
                    onChanged: (v) => _send = v.trim().toUpperCase(),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: _currencyPicker(
                    label: t('tools.receive_currency'),
                    value: _receive,
                    fallbackController: _receiveController,
                    currencies: currencies,
                    onChanged: (v) => _receive = v.trim().toUpperCase(),
                  ),
                ),
                const SizedBox(width: 12),
                ElevatedButton(
                  onPressed: _doSearch,
                  child: Text(t('tools.search')),
                ),
              ],
            ),
          ),
          Expanded(
            child: rates.when(
              data: (items) => items.isEmpty
                  ? Center(child: Text(t('tools.no_rates')))
                  : ListView.builder(
                      itemCount: sortedRates(items).length,
                      itemBuilder: (_, i) {
                        final r = sortedRates(items)[i];
                        return ListTile(
                          title: Text(r.providerName ?? t('tools.unknown_provider')),
                          subtitle: Text(
                              '${currencyChip(r.sendCurrency)} → ${currencyChip(r.receiveCurrency)}'),
                          trailing: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            crossAxisAlignment: CrossAxisAlignment.end,
                            children: [
                              Text(formatRate(r.rate)),
                              if (r.isBenchmark)
                                Text(
                                  t('tools.mid_market'),
                                  style: Theme.of(context).textTheme.labelSmall,
                                ),
                            ],
                          ),
                        );
                      },
                    ),
              loading: () => const LoadingShimmer(),
              error: (e, _) => JcErrorWidget(
                error: e,
                onRetry: () {
                  if (_search != null) {
                    ref.invalidate(remittanceRatesProvider(_search!));
                  }
                },
              ),
            ),
          ),
        ],
      ),
    );
  }
}
