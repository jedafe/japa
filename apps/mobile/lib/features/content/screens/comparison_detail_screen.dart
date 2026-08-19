import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/providers/content_provider.dart';

class ComparisonDetailScreen extends ConsumerWidget {
  final String id;
  const ComparisonDetailScreen({super.key, required this.id});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(comparisonDetailProvider(id));
    return Scaffold(
      appBar: AppBar(title: const Text('Comparison')),
      body: state.when(
        data: (item) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(item['title'] ?? '', style: JcTypography.heading),
              const SizedBox(height: 8),
              ...((item['items'] as List? ?? [])).map<Widget>(
                (row) => ListTile(
                  title: Text(row['label']?.toString() ?? ''),
                  trailing: Text(row['value']?.toString() ?? ''),
                ),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e,
            onRetry: () => ref.invalidate(comparisonDetailProvider(id))),
      ),
    );
  }
}
