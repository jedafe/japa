import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/providers/social_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';
import 'package:japaconnect_mobile/features/social/services/report_service.dart';
import 'package:japaconnect_mobile/features/social/widgets/report_sheet.dart';

class MessageThreadScreen extends ConsumerStatefulWidget {
  final String id;
  const MessageThreadScreen({super.key, required this.id});

  @override
  ConsumerState<MessageThreadScreen> createState() =>
      _MessageThreadScreenState();
}

class _MessageThreadScreenState extends ConsumerState<MessageThreadScreen> {
  late final TextEditingController _textController;

  @override
  void initState() {
    super.initState();
    _textController = TextEditingController();
  }

  @override
  void dispose() {
    _textController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    ref.watch(socialRealtimeSyncProvider); // keep realtime + push listener alive
    final messages = ref.watch(conversationMessagesProvider(widget.id));
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('social.message_thread'))),
      body: messages.when(
        data: (items) => Column(
          children: [
            Expanded(
              child: ListView.builder(
                itemCount: items.length,
                itemBuilder: (_, i) => ListTile(
                      title: Text(items[i].text),
                      trailing: IconButton(
                        icon: const Icon(Icons.flag_outlined, size: 18),
                        tooltip: t('report.action'),
                        onPressed: () => showReportSheet(
                          context,
                          onSubmit: (reason, notes) => ref
                              .read(reportServiceProvider)
                              .reportDMMessage(widget.id, items[i].id,
                                  reason: reason, notes: notes),
                        ),
                      ),
                    ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Expanded(
                      child: JcTextField(
                          label: t('social.message_label'), controller: _textController)),
                  JcIconButton(
                    icon: Icons.send,
                    onPressed: () async {
                      final text = _textController.text;
                      if (text.isEmpty) return;
                      try {
                        await ref
                            .read(socialServiceProvider)
                            .sendConversationMessage(widget.id, text);
                        _textController.clear();
                        ref.invalidate(
                            conversationMessagesProvider(widget.id));
                      } catch (e) {
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(content: Text(t('errors.failed_send', {'error': e.toString()}))),
                          );
                        }
                      }
                    },
                  ),
                ],
              ),
            ),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e,
            onRetry: () =>
                ref.invalidate(conversationMessagesProvider(widget.id))),
      ),
    );
  }
}
