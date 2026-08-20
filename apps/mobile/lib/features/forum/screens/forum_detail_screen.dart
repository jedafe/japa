import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ForumDetailScreen extends ConsumerWidget {
  final String slug;
  const ForumDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(forumThreadProvider(slug));
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('forum.thread'))),
      body: state.when(
        data: (thread) => Column(
          children: [
            ListTile(title: Text(thread.title)),
            Expanded(
              child: ListView.builder(
                itemCount: thread.replies?.length ?? 0,
                itemBuilder: (_, i) =>
                    ListTile(title: Text(thread.replies![i].body)),
              ),
            ),
            JcButton(
              label: t('forum.reply'),
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
