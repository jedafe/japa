import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';

class BookmarksScreen extends ConsumerWidget {
  const BookmarksScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(bookmarksProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Bookmarks')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) =>
              ListTile(title: Text(items[i]['title'] ?? '')),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e,
            onRetry: () => ref.invalidate(bookmarksProvider)),
      ),
    );
  }
}
