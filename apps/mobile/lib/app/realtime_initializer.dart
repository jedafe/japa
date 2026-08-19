import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

class RealtimeInitializer extends ConsumerStatefulWidget {
  final Widget child;
  const RealtimeInitializer({super.key, required this.child});

  @override
  ConsumerState<RealtimeInitializer> createState() => _RealtimeInitializerState();
}

class _RealtimeInitializerState extends ConsumerState<RealtimeInitializer> {
  @override
  void initState() {
    super.initState();
    ref.listen<bool>(isAuthenticatedProvider, (prev, next) {
      final socket = ref.read(socketServiceProvider);
      if (next) {
        socket.connect().catchError((_) {}); // reconnect scheduled internally
      } else {
        socket.disconnect();
      }
    });
  }

  @override
  void dispose() {
    ref.read(socketServiceProvider).disconnect();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
