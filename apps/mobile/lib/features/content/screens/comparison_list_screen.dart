import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/providers/content_provider.dart';

class ComparisonListScreen extends ConsumerWidget {
  const ComparisonListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(comparisonListProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Comparisons')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => EntityCard(
            item: items[i],
            onTap: () => context.push('/comparisons/${items[i]['id']}'),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(comparisonListProvider)),
      ),
    );
  }
}
