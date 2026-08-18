import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

class PushInitializer extends ConsumerStatefulWidget {
  final Widget child;
  const PushInitializer({super.key, required this.child});

  @override
  ConsumerState<PushInitializer> createState() => _PushInitializerState();
}

class _PushInitializerState extends ConsumerState<PushInitializer> {
  StreamSubscription<Object?>? _pushSub;

  @override
  void initState() {
    super.initState();
    final push = ref.read(pushServiceProvider);
    push.initialize();
    _pushSub = push.foregroundMessages.listen((_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('New notification')),
      );
    });
    ref.listen<bool>(isAuthenticatedProvider, (prev, next) {
      if (next == true && prev != true) {
        ref.read(pushServiceProvider).initialize();
      }
    });
  }

  @override
  void dispose() {
    _pushSub?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
