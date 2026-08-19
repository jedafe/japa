import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';

class ForumReplyScreen extends ConsumerWidget {
  final String slug;
  const ForumReplyScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final body = TextEditingController();
    return Scaffold(
      appBar: AppBar(title: const Text('Reply')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            JcTextField(label: 'Reply', controller: body),
            JcButton(
              label: 'Post',
              onPressed: () async {
                final service = ref.read(forumServiceProvider);
                final thread =
                    await ref.read(forumThreadProvider(slug).future);
                await service.postReply(thread['id'] as String, body.text);
                if (context.mounted) context.pop();
              },
            ),
          ],
        ),
      ),
    );
  }
}
