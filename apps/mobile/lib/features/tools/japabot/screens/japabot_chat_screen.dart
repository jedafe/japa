import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/providers/japabot_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class JapaBotChatScreen extends ConsumerStatefulWidget {
  final String conversationId;
  final String? title;

  const JapaBotChatScreen({
    super.key,
    required this.conversationId,
    this.title,
  });

  @override
  ConsumerState<JapaBotChatScreen> createState() => _JapaBotChatScreenState();
}

class _JapaBotChatScreenState extends ConsumerState<JapaBotChatScreen> {
  final _controller = TextEditingController();
  bool _sending = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(japabotMessagesProvider(widget.conversationId));
    final t = ref.watch(tProvider);

    return Scaffold(
      appBar: AppBar(title: Text(widget.title ?? t('tools.japabot'))),
      body: Column(
        children: [
          Expanded(
            child: state.when(
              data: (messages) => ListView.builder(
                reverse: true,
                padding: const EdgeInsets.all(16),
                itemCount: messages.length,
                itemBuilder: (_, i) {
                  final msg = messages[messages.length - 1 - i];
                  final isUser = msg.role == 'user';
                  return Align(
                    alignment:
                        isUser ? Alignment.centerRight : Alignment.centerLeft,
                    child: Container(
                      margin: const EdgeInsets.only(bottom: 8),
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: isUser
                            ? Theme.of(context).colorScheme.primary
                            : Theme.of(context).colorScheme.surfaceContainerHighest,
                        borderRadius: BorderRadius.circular(12),
                      ),
                      constraints: BoxConstraints(
                          maxWidth: MediaQuery.of(context).size.width * 0.75),
                      child: Text(
                        msg.text,
                        style: TextStyle(
                          color: isUser
                              ? Theme.of(context).colorScheme.onPrimary
                              : Theme.of(context).colorScheme.onSurface,
                        ),
                      ),
                    ),
                  );
                },
              ),
              loading: () => const LoadingShimmer(),
              error: (e, _) => JcErrorWidget(
                error: e,
                onRetry: () => ref.invalidate(
                    japabotMessagesProvider(widget.conversationId)),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(8),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _controller,
                    enabled: !_sending,
                    decoration: InputDecoration(
                      hintText: t('tools.message_hint'),
                      border: const OutlineInputBorder(),
                    ),
                    onSubmitted: (_) => _send(),
                  ),
                ),
                const SizedBox(width: 8),
                if (_sending)
                  const Padding(
                    padding: EdgeInsets.all(8),
                    child: CircularProgressIndicator(),
                  )
                else
                  IconButton(
                    icon: const Icon(Icons.send),
                    onPressed: _send,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _send() async {
    final text = _controller.text;
    if (text.trim().isEmpty) return;
    setState(() => _sending = true);
    _controller.clear();
    try {
      await ref.read(japabotServiceProvider).sendMessage(
            widget.conversationId,
            text.trim(),
          );
      ref.invalidate(japabotMessagesProvider(widget.conversationId));
      ref.invalidate(japabotConversationsProvider);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }
}
