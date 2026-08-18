import 'package:flutter_riverpod/flutter_riverpod.dart';

class IsAuthenticatedNotifier extends Notifier<bool> {
  @override
  bool build() => false;

  void set(bool value) => state = value;
}

final isAuthenticatedProvider =
    NotifierProvider<IsAuthenticatedNotifier, bool>(
  IsAuthenticatedNotifier.new,
);
