import 'dart:convert';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class AppLocalizations {
  final Map<String, String> _strings;
  AppLocalizations(this._strings);

  String t(String key, [Map<String, String>? params]) {
    var value = _strings[key] ?? key;
    if (params != null) {
      params.forEach((k, v) => value = value.replaceAll('{$k}', v));
    }
    return value;
  }

  static Future<AppLocalizations> load(String locale) async {
    final json = await rootBundle.loadString('lib/i18n/$locale.json');
    final map = Map<String, String>.from(jsonDecode(json));
    return AppLocalizations(map);
  }
}

class LocaleNotifier extends Notifier<String> {
  @override
  String build() => 'en';

  void setLocale(String locale) {
    state = locale;
  }
}

final localeProvider =
    NotifierProvider<LocaleNotifier, String>(LocaleNotifier.new);

final localizationsProvider =
    FutureProvider.family<AppLocalizations, String>((ref, locale) async {
  return AppLocalizations.load(locale);
});

final tProvider = Provider<String Function(String, [Map<String, String>?])>(
    (ref) {
  final locale = ref.watch(localeProvider);
  final locAsync = ref.watch(localizationsProvider(locale));
  return locAsync.when(
    data: (loc) => loc.t,
    loading: () => (key, [params]) => key,
    error: (_, _) => (key, [params]) => key,
  );
});
