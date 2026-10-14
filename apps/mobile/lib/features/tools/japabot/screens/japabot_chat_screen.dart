import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/providers/japabot_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class JapaBotChatScreen extends ConsumerStatefulWidget {
  /// Route param. 'new' is the sentinel for a thread that doesn't exist yet —
  /// the server creates it on the first streamed turn and returns the id in
  /// the `conversation` event.
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

  /// Messages fetched/seeded from the server, plus turns appended locally.
  /// Null until the initial provider load resolves.
  List<JapaBotMessage>? _messages;

  /// The thread id the next send targets — starts as the route param, then is
  /// adopted from the `conversation` event when a new thread is created.
  String? _convId;

  /// In-flight turn state (ADR-130 SSE contract):
  /// `delta` chunks accumulate into [_streamed]; `final_response` replaces the
  /// transient bubble with the authoritative reply.
  StreamSubscription<Map<String, dynamic>>? _sub;

  /// Monotonic send-generation — bumped on every send and on cancel. Events
  /// from a stale (cancelled or superseded) subscription carry the old gen
  /// and are dropped, so a buffered `final_response` can't double-append
  /// after the cancel path already promoted the partial.
  int _sendGen = 0;
  String _streamed = '';
  List<String> _sources = const [];
  String? _status;
  String? _error;
  String? _sessionId;
  bool _sending = false;

  /// Per-message tools toggle (ADR-130 §capabilities) — shown only when the
  /// caller's catalog is non-empty.
  bool _toolsEnabled = true;

  bool get _isNew => _convId == null || _convId == 'new';

  @override
  void initState() {
    super.initState();
    _convId = widget.conversationId;
  }

  @override
  void dispose() {
    _sub?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state =
        ref.watch(japabotMessagesProvider(widget.conversationId));
    final caps = ref.watch(japabotCapabilitiesProvider);
    final t = ref.watch(tProvider);

    // Seed the local transcript once the initial fetch resolves. Afterwards
    // _messages owns the truth — streamed turns are appended locally and the
    // provider is only invalidated to refresh the conversations list.
    _messages ??= state.value;

    final showToolsToggle = (caps.value?.isNotEmpty ?? false);

    return Scaffold(
      appBar: AppBar(
        title: Text(widget.title ?? t('tools.japabot')),
        actions: [
          if (showToolsToggle)
            IconButton(
              tooltip: _toolsEnabled
                  ? t('tools.tools_on')
                  : t('tools.tools_off'),
              icon: Icon(_toolsEnabled
                  ? Icons.construction
                  : Icons.construction_outlined),
              onPressed: _sending
                  ? null
                  : () => setState(() => _toolsEnabled = !_toolsEnabled),
            ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: state.when(
              data: (_) => _buildList(t),
              loading: () =>
                  _messages == null ? const LoadingShimmer() : _buildList(t),
              error: (e, _) => _messages != null
                  ? _buildList(t)
                  : JcErrorWidget(
                      error: e,
                      onRetry: () => ref.invalidate(
                          japabotMessagesProvider(widget.conversationId)),
                    ),
            ),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
              child: Text(
                _error!,
                style:
                    TextStyle(color: Theme.of(context).colorScheme.error),
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
                  IconButton(
                    icon: const Icon(Icons.stop_circle_outlined),
                    tooltip: t('tools.stop'),
                    onPressed: _cancel,
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

  Widget _buildList(String Function(String, [Map<String, String>?]) t) {
    final msgs = _messages ?? const <JapaBotMessage>[];
    final count = msgs.length + (_sending ? 1 : 0);
    return ListView.builder(
      reverse: true,
      padding: const EdgeInsets.all(16),
      itemCount: count,
      itemBuilder: (_, i) {
        // index 0 (bottom of the reversed list) is the in-flight bubble.
        if (_sending && i == 0) return _buildPendingBubble(t);
        final idx = msgs.length - 1 - (i - (_sending ? 1 : 0));
        final msg = msgs[idx];
        return _buildBubble(
          text: msg.text,
          isUser: msg.role == 'user',
          sources: msg.sources,
          t: t,
        );
      },
    );
  }

  Widget _buildPendingBubble(String Function(String, [Map<String, String>?]) t) {
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: Theme.of(context).colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(12),
        ),
        constraints:
            BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.75),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            if (_streamed.isEmpty)
              const SizedBox(
                width: 16,
                height: 16,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            else
              Text(
                _streamed,
                style: TextStyle(
                  color: Theme.of(context).colorScheme.onSurface,
                ),
              ),
            if (_status != null)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text(
                  _status!,
                  style: Theme.of(context).textTheme.labelSmall,
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildBubble({
    required String text,
    required bool isUser,
    required List<String> sources,
    required String Function(String, [Map<String, String>?]) t,
  }) {
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: isUser
              ? Theme.of(context).colorScheme.primary
              : Theme.of(context).colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(12),
        ),
        constraints:
            BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.75),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              text,
              style: TextStyle(
                color: isUser
                    ? Theme.of(context).colorScheme.onPrimary
                    : Theme.of(context).colorScheme.onSurface,
              ),
            ),
            if (sources.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text(
                  '${t('tools.sources')}: ${sources.take(3).join(', ')}',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                        color: isUser
                            ? Theme.of(context).colorScheme.onPrimary
                            : Theme.of(context)
                                .colorScheme
                                .onSurfaceVariant,
                      ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _send() async {
    final text = _controller.text.trim();
    if (text.isEmpty || _sending) return;

    _controller.clear();
    setState(() {
      _sending = true;
      _streamed = '';
      _sources = const [];
      _status = null;
      _error = null;
      _sessionId = null;
      (_messages ??= []).add(JapaBotMessage.user(
        text: text,
        conversationId: _isNew ? null : _convId,
      ));
    });

    final service = ref.read(japabotServiceProvider);
    final gen = ++_sendGen;
    var gotFinal = false;

    _sub = service
        .streamMessage(
          conversationId: _convId,
          message: text,
          toolsEnabled: _toolsEnabled,
        )
        .listen(
      (ev) {
        if (!mounted || gen != _sendGen) return;
        setState(() {
          switch (ev['type'] as String? ?? '') {
            case 'session_start':
              _sessionId = ev['session_id'] as String?;
            case 'conversation':
              _convId = ev['conversation_id'] as String?;
            case 'delta':
              _streamed += (ev['content'] as String? ?? '');
            case 'tool_call_start':
              _status = ev['tool_name'] as String?;
              // Deltas so far were preamble from an intermediate iteration —
              // transient, never persisted. Clear so the final iteration
              // streams into a clean bubble.
              _streamed = '';
            case 'tool_call_result':
              _status = null;
            case 'progress':
              _status = ev['message'] as String?;
            case 'sources':
              final raw = ev['sources'] as List<dynamic>? ?? const [];
              _sources = raw
                  .map((e) {
                    if (e is String) return e;
                    if (e is Map<String, dynamic>) {
                      return (e['title'] ?? e['url'] ?? '') as String;
                    }
                    return '';
                  })
                  .where((s) => s.isNotEmpty)
                  .toList();
            case 'final_response':
              gotFinal = true;
              _messages!.add(JapaBotMessage(
                text: (ev['content'] as String? ?? _streamed),
                role: 'assistant',
                conversationId: _convId,
                createdAt: DateTime.now(),
                sources: _sources,
              ));
              _streamed = '';
              _status = null;
            case 'error':
              // Promote the streamed partial into the transcript so what
              // arrived before the failure stays visible above the error.
              if (_streamed.isNotEmpty) {
                _messages?.add(JapaBotMessage(
                  text: _streamed,
                  role: 'assistant',
                  conversationId: _convId,
                  createdAt: DateTime.now(),
                  sources: _sources,
                ));
                _streamed = '';
              }
              _error = ev['error'] as String?;
          }
        });
      },
      onError: (Object e) {
        if (!mounted || gen != _sendGen) return;
        setState(() => _error = e.toString());
      },
      onDone: () {
        if (!mounted || gen != _sendGen) return;
        setState(() {
          // The stream closed without a final_response (cancel, transport
          // drop). Promote any streamed partial so the text isn't lost.
          if (!gotFinal && _streamed.isNotEmpty) {
            _messages!.add(JapaBotMessage(
              text: _streamed,
              role: 'assistant',
              conversationId: _convId,
              createdAt: DateTime.now(),
              sources: _sources,
            ));
          }
          _sending = false;
          _streamed = '';
          _status = null;
          _sub = null;
        });
        ref.invalidate(japabotConversationsProvider);
        if (!_isNew) {
          ref.invalidate(japabotMessagesProvider(_convId!));
        }
      },
    );
  }

  Future<void> _cancel() async {
    final sub = _sub;
    _sub = null;
    _sendGen++; // drop any events buffered by the dying subscription
    await sub?.cancel();
    final sessionId = _sessionId;
    if (sessionId != null && sessionId.isNotEmpty) {
      // Best-effort — the loop checks its cancel flag at each iteration.
      try {
        await ref.read(japabotServiceProvider).cancelSession(sessionId);
      } catch (_) {}
    }
    // A cancelled subscription never delivers onDone — reset the turn state
    // here so the input isn't stuck on the stop button, and promote any
    // streamed partial into the transcript instead of dropping it.
    if (!mounted) return;
    setState(() {
      if (_streamed.isNotEmpty) {
        _messages?.add(JapaBotMessage(
          text: _streamed,
          role: 'assistant',
          conversationId: _isNew ? null : _convId,
          createdAt: DateTime.now(),
          sources: _sources,
        ));
      }
      _sending = false;
      _streamed = '';
      _status = null;
    });
    ref.invalidate(japabotConversationsProvider);
    if (!_isNew) {
      ref.invalidate(japabotMessagesProvider(_convId!));
    }
  }
}
