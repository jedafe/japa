import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/providers/content_provider.dart';

class BlogListScreen extends ConsumerWidget {
  const BlogListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(blogListProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Blog')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => EntityCard(
            item: items[i],
            onTap: () => context.push('/blog/${items[i]['slug']}'),
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) =>
            JcErrorWidget(error: e, onRetry: () => ref.invalidate(blogListProvider)),
      ),
    );
  }
}
