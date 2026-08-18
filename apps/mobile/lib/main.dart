import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:firebase_core/firebase_core.dart';
import 'app/theme.dart';
import 'app/router.dart';
import 'app/push_initializer.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (defaultTargetPlatform == TargetPlatform.android ||
      defaultTargetPlatform == TargetPlatform.iOS) {
    await Firebase.initializeApp();
  }
  runApp(const ProviderScope(
      child: PushInitializer(child: JapaConnectApp())));
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
