import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

class AuthInitializer extends ConsumerStatefulWidget {
  final Widget child;
  const AuthInitializer({super.key, required this.child});

  @override
  ConsumerState<AuthInitializer> createState() => _AuthInitializerState();
}

class _AuthInitializerState extends ConsumerState<AuthInitializer> {
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _restore();
  }

  Future<void> _restore() async {
    final auth = ref.read(authServiceProvider);
    final restored = await auth.tryRestore();
    if (restored && mounted) {
      ref.read(isAuthenticatedProvider.notifier).set(true);
    }
    if (mounted) {
      setState(() => _ready = true);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!_ready) {
      return const Material(
        child: Center(child: CircularProgressIndicator()),
      );
    }
    return widget.child;
  }
}
