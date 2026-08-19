import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/scholarships/providers/scholarships_provider.dart';

class ScholarshipsScreen extends ConsumerWidget {
  const ScholarshipsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(scholarshipsProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Scholarships')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => ListTile(
            title: Text(items[i]['title'] ?? ''),
            trailing: IconButton(
              icon: Icon(items[i]['is_saved'] == true
                  ? Icons.bookmark
                  : Icons.bookmark_border),
              onPressed: () {
                final service = ref.read(scholarshipsServiceProvider);
                service.save(items[i]['id'] as String, true);
              },
            ),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(scholarshipsProvider),
        ),
      ),
    );
  }
}
