import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../i18n/app_localizations.dart';

class AppShell extends ConsumerWidget {
  final Widget child;

  const AppShell({super.key, required this.child});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    return Scaffold(
      body: child,
      bottomNavigationBar: BottomNavigationBar(
        currentIndex: _currentIndex(context),
        onTap: (i) => _onTap(context, i),
        items: [
          BottomNavigationBarItem(icon: const Icon(Icons.home), label: t('nav.home')),
          BottomNavigationBarItem(icon: const Icon(Icons.group), label: t('nav.spaces')),
          BottomNavigationBarItem(icon: const Icon(Icons.add_circle), label: t('nav.compose')),
          BottomNavigationBarItem(icon: const Icon(Icons.notifications), label: t('nav.notifications')),
          BottomNavigationBarItem(icon: const Icon(Icons.person), label: t('nav.profile')),
        ],
      ),
    );
  }

  int _currentIndex(BuildContext context) {
    final path = GoRouterState.of(context).uri.path;
    if (path == '/' || path == '/discover' || path == '/trending') return 0;
    if (path.startsWith('/spaces')) return 1;
    if (path.startsWith('/notifications')) return 3;
    if (path.startsWith('/profile')) return 4;
    return 0;
  }

  void _onTap(BuildContext context, int index) {
    switch (index) {
      case 0:
        context.go('/');
        break;
      case 1:
        context.go('/spaces');
        break;
      case 2:
        context.push('/compose');
        break;
      case 3:
        context.go('/notifications');
        break;
      case 4:
        context.go('/profile');
        break;
    }
  }
}
