import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'app/theme.dart';
import 'app/router.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ProviderScope(child: JapaConnectApp()));
}

class JapaConnectApp extends ConsumerWidget {
  const JapaConnectApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      title: 'JapaConnect',
      theme: ref.watch(jcThemeProvider),
      routerConfig: ref.watch(jcRouterProvider),
    );
  }
}
