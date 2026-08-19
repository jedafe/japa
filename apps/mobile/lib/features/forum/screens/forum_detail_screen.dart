import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';

class ForumDetailScreen extends ConsumerWidget {
  final String slug;
  const ForumDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(forumThreadProvider(slug));
    return Scaffold(
      appBar: AppBar(title: const Text('Thread')),
      body: state.when(
        data: (thread) => Column(
          children: [
            ListTile(title: Text(thread['title'] ?? '')),
            Expanded(
              child: ListView.builder(
                itemCount: (thread['replies'] as List? ?? []).length,
                itemBuilder: (_, i) =>
                    ListTile(title: Text(thread['replies'][i]['body'] ?? '')),
              ),
            ),
            JcButton(
              label: 'Reply',
              onPressed: () => context.push('/forum/$slug/reply'),
            ),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(forumThreadProvider(slug)),
        ),
      ),
    );
  }
}
