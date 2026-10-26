import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';
import 'package:japaconnect_mobile/features/social/services/report_service.dart';
import 'package:japaconnect_mobile/features/social/widgets/report_sheet.dart';
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
            _ThreadHeader(thread: thread),
            Expanded(
              child: _ReplyList(thread: thread),
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

class _ThreadHeader extends ConsumerWidget {
  final ForumThread thread;
  const _ThreadHeader({required this.thread});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final service = ref.watch(forumServiceProvider);
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(thread.title, style: Theme.of(context).textTheme.titleMedium),
          if (thread.bodyText?.isNotEmpty ?? false)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(thread.bodyText!, style: Theme.of(context).textTheme.bodyMedium),
            ),
          const SizedBox(height: 8),
          Row(
            children: [
              Text('${thread.voteScore} ${t('forum.votes')}'),
              const SizedBox(width: 16),
              Text('${thread.viewCount} ${t('forum.views')}'),
              if (thread.replyCount > 0) ...[
                const SizedBox(width: 16),
                Text('${thread.replyCount} ${t('forum.replies')}'),
              ],
            ],
          ),
          if (thread.solved)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Chip(label: Text(t('forum.solved'))),
            ),
          Row(
            children: [
              IconButton(
                icon: const Icon(Icons.arrow_upward),
                onPressed: () => service.vote(thread.id, 'thread', 1),
              ),
              IconButton(
                icon: const Icon(Icons.arrow_downward),
                onPressed: () => service.vote(thread.id, 'thread', -1),
              ),
              if (thread.canSolve ?? false)
                IconButton(
                  icon: const Icon(Icons.check_circle),
                  onPressed: () => service.solveThread(thread.id),
                ),
              IconButton(
                icon: const Icon(Icons.bookmark),
                onPressed: () => service.bookmarkThread(thread.id),
              ),
              if (thread.contentId != null)
                IconButton(
                  icon: const Icon(Icons.flag_outlined),
                  tooltip: t('report.action'),
                  onPressed: () => showReportSheet(
                    context,
                    onSubmit: (reason, notes) => ref
                        .read(reportServiceProvider)
                        .reportContent(thread.contentId!, reason: reason, notes: notes),
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _ReplyList extends StatelessWidget {
  final ForumThread thread;
  const _ReplyList({required this.thread});

  @override
  Widget build(BuildContext context) {
    final replies = thread.replies ?? [];
    return ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      itemCount: replies.length,
      itemBuilder: (_, i) => _ReplyTile(reply: replies[i]),
    );
  }
}

class _ReplyTile extends ConsumerWidget {
  final ForumReply reply;
  const _ReplyTile({required this.reply});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final service = ref.watch(forumServiceProvider);
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 4),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              reply.isAnonymous
                  ? t('forum.anonymous')
                  : reply.authorDisplayName ?? reply.authorUsername ?? t('forum.member'),
              style: Theme.of(context).textTheme.labelSmall,
            ),
            const SizedBox(height: 4),
            Text(reply.bodyText ?? ''),
            const SizedBox(height: 8),
            Row(
              children: [
                IconButton(
                  icon: const Icon(Icons.arrow_upward, size: 18),
                  onPressed: () => service.vote(reply.id, 'reply', 1),
                ),
                Text('${reply.voteScore}'),
                IconButton(
                  icon: const Icon(Icons.arrow_downward, size: 18),
                  onPressed: () => service.vote(reply.id, 'reply', -1),
                ),
                IconButton(
                  icon: const Icon(Icons.flag_outlined, size: 18),
                  tooltip: t('report.action'),
                  onPressed: () => showReportSheet(
                    context,
                    onSubmit: (reason, notes) => ref
                        .read(reportServiceProvider)
                        .reportForumReply(reply.id, reason: reason, notes: notes),
                  ),
                ),
                const Spacer(),
                if (reply.viewCount > 0)
                  Text('${reply.viewCount} ${t('forum.views')}'),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
