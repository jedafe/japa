import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/providers/forum_provider.dart';

class ForumReplyScreen extends ConsumerStatefulWidget {
  final String slug;
  const ForumReplyScreen({super.key, required this.slug});

  @override
  ConsumerState<ForumReplyScreen> createState() => _ForumReplyScreenState();
}

class _ForumReplyScreenState extends ConsumerState<ForumReplyScreen> {
  late final TextEditingController _bodyController;
  bool _submitting = false;

  @override
  void initState() {
    super.initState();
    _bodyController = TextEditingController();
  }

  @override
  void dispose() {
    _bodyController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Reply')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            JcTextField(label: 'Reply', controller: _bodyController),
            JcButton(
              label: 'Post',
              onPressed: _submitting
                  ? null
                  : () async {
                      setState(() => _submitting = true);
                      try {
                        final service = ref.read(forumServiceProvider);
                        final thread = await ref
                            .read(forumThreadProvider(widget.slug).future);
                        await service.postReply(
                            thread['id'] as String, _bodyController.text);
                        if (context.mounted) context.pop();
                      } catch (e) {
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(content: Text('Failed to post reply: $e')),
                          );
                        }
                      } finally {
                        if (mounted) setState(() => _submitting = false);
                      }
                    },
            ),
          ],
        ),
      ),
    );
  }
}
