import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/remittance/providers/remittance_provider.dart';

class RemittanceScreen extends ConsumerWidget {
  const RemittanceScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(remittanceProvidersProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Remittance Rates')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => ListTile(
            title: Text(items[i]['name'] ?? ''),
            subtitle: Text('Rate: ${items[i]['rate'] ?? ''}'),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(remittanceProvidersProvider),
        ),
      ),
    );
  }
}
