import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ForumListScreen extends ConsumerWidget {
  const ForumListScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(forumListProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('forum.title'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) {
            final item = items[i];
            return ListTile(
              title: Text(item.title),
              subtitle: item.bodyText?.isNotEmpty ?? false
                  ? Text(
                      item.bodyText!,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    )
                  : null,
              trailing: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text('${item.voteScore}'),
                  const SizedBox(width: 8),
                  Text('${item.replyCount}'),
                  if (item.solved) ...[
                    const SizedBox(width: 8),
                    Icon(Icons.check_circle, size: 16, color: Theme.of(context).colorScheme.primary),
                  ],
                ],
              ),
              onTap: () => context.push('/forum/${item.slug}'),
            );
          },
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(forumListProvider),
        ),
      ),
    );
  }
}
