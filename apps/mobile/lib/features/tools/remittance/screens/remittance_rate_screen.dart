import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
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
  ({String send, String receive})? _search;

  @override
  void dispose() {
    _sendController.dispose();
    _receiveController.dispose();
    super.dispose();
  }

  void _doSearch() {
    setState(() {
      _search = (send: _sendController.text.trim().toUpperCase(),
          receive: _receiveController.text.trim().toUpperCase());
    });
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final rates = _search != null
        ? ref.watch(remittanceRatesProvider(_search!))
        : const AsyncData<List<RemittanceRate>>([]);

    return Scaffold(
      appBar: AppBar(title: Text(t('tools.remittance_rates'))),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _sendController,
                    decoration: InputDecoration(
                      labelText: t('tools.send_currency'),
                      border: const OutlineInputBorder(),
                    ),
                    textCapitalization: TextCapitalization.characters,
                    onSubmitted: (_) => _doSearch(),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: TextField(
                    controller: _receiveController,
                    decoration: InputDecoration(
                      labelText: t('tools.receive_currency'),
                      border: const OutlineInputBorder(),
                    ),
                    textCapitalization: TextCapitalization.characters,
                    onSubmitted: (_) => _doSearch(),
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
                      itemCount: items.length,
                      itemBuilder: (_, i) => ListTile(
                        title: Text(items[i].providerName ?? t('tools.unknown_provider')),
                        subtitle: Text(
                            '${items[i].sendCurrency} → ${items[i].receiveCurrency}'),
                        trailing: Text('${items[i].rate}'),
                      ),
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
