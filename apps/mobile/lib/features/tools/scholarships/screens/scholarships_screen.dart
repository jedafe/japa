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
          itemBuilder: (_, i) {
            final isSaved = items[i]['is_saved'] == true;
            return ListTile(
              title: Text(items[i]['title'] ?? ''),
              trailing: IconButton(
                icon: Icon(
                    isSaved ? Icons.bookmark : Icons.bookmark_border),
                onPressed: () {
                  final service = ref.read(scholarshipsServiceProvider);
                  final id = items[i]['id'] as String;
                  if (isSaved) {
                    service.unsave(id);
                  } else {
                    service.save(id);
                  }
                },
              ),
            );
          },
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
