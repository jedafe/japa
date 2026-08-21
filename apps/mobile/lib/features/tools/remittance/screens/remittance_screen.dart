import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/remittance/providers/remittance_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class RemittanceScreen extends ConsumerWidget {
  const RemittanceScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(remittanceProvidersProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(t('tools.remittance_rates')),
        actions: [
          IconButton(
            icon: const Icon(Icons.trending_up),
            tooltip: t('tools.view_rates'),
            onPressed: () => context.push('/tools/remittance/rates'),
          ),
        ],
      ),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => ListTile(
            title: Text(items[i].name),
            subtitle: items[i].websiteUrl != null
                ? Text(items[i].websiteUrl!)
                : null,
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
