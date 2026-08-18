import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:japaconnect_core/auth/token_store.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/auth/auth_state.dart';
import 'api_error.dart';

final tokenStoreProvider = Provider<TokenStore>((ref) => TokenStore());

final jcApiClientProvider = Provider<JcApiClient>((ref) {
  final tokenStore = ref.watch(tokenStoreProvider);
  final deviceId = ref.watch(deviceIdProvider);
  final baseUrl = const String.fromEnvironment(
    'JC_API_BASE_URL',
    defaultValue: 'http://localhost:8080/api/v1',
  );
  final wsUrl = const String.fromEnvironment(
    'JC_WS_URL',
    defaultValue: '',
  );
  return JcApiClient(
    baseUrl: baseUrl,
    wsUrl: wsUrl,
    tokenStore: tokenStore,
    deviceId: deviceId,
    onUnauthenticated: () =>
        ref.read(isAuthenticatedProvider.notifier).set(false),
  );
});

class JcApiClient {
  final String baseUrl;
  final String wsUrl;
  final TokenStore tokenStore;
  final DeviceId deviceId;
  final VoidCallback? onUnauthenticated;
  String? _accessToken;
  final http.Client _http = http.Client();

  JcApiClient({
    required this.baseUrl,
    this.wsUrl = '',
    required this.tokenStore,
    required this.deviceId,
    this.onUnauthenticated,
  });

  String get effectiveWsUrl {
    if (wsUrl.isNotEmpty) return wsUrl;
    final base = Uri.parse(baseUrl).resolve('/').toString();
    return base
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://')
        .replaceFirst(RegExp(r'/?$'), '/ws');
  }

  String? get accessToken => _accessToken;

  void setAccessToken(String token) => _accessToken = token;
  void clearAccessToken() => _accessToken = null;

  Future<Map<String, String>> _buildHeaders({
    bool includeAuth = true,
  }) async {
    final headers = <String, String>{
      'X-Device-Id': await deviceId.value,
    };
    if (includeAuth && _accessToken != null) {
      headers['Authorization'] = 'Bearer $_accessToken';
    }
    return headers;
  }

  Future<T> get<T>(
    String path, {
    Map<String, dynamic>? query,
    required T Function(dynamic) parse,
  }) async {
    return _request<T>('GET', path, query: query, parse: parse);
  }

  Future<T> post<T>(
    String path, {
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
  }) async {
    return _request<T>('POST', path, body: body, parse: parse);
  }

  Future<T> patch<T>(
    String path, {
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
  }) async {
    return _request<T>('PATCH', path, body: body, parse: parse);
  }

  Future<T> put<T>(
    String path, {
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
  }) async {
    return _request<T>('PUT', path, body: body, parse: parse);
  }

  Future<void> delete(String path) async {
    await _request<void>('DELETE', path, parse: (_) {});
  }

  Future<T> _request<T>(
    String method,
    String path, {
    Map<String, dynamic>? query,
    Map<String, dynamic>? body,
    required T Function(dynamic) parse,
    bool includeAuth = true,
    bool isRetry = false,
  }) async {
    var uri = Uri.parse('$baseUrl$path');
    if (query != null) {
      uri = uri.replace(
        queryParameters:
            query.map((k, v) => MapEntry(k, v.toString())),
      );
    }

    final headers = await _buildHeaders(includeAuth: includeAuth);
    headers['Content-Type'] = 'application/json';

    final request = http.Request(method, uri);
    request.headers.addAll(headers);
    if (body != null) {
      request.body = jsonEncode(body);
    }

    final streamedResponse = await _http.send(request);
    final response = await http.Response.fromStream(streamedResponse);

    if (response.statusCode == 401) {
      if (!includeAuth || isRetry) {
        onUnauthenticated?.call();
        throw JcApiError(
          code: 'unauthenticated',
          message: 'Session expired',
          status: 401,
        );
      }
      final refreshed = await _refresh();
      if (!refreshed) {
        onUnauthenticated?.call();
        throw JcApiError(
          code: 'unauthenticated',
          message: 'Session expired',
          status: 401,
        );
      }
      return _request<T>(
        method,
        path,
        query: query,
        body: body,
        parse: parse,
        isRetry: true,
      );
    }

    if (response.statusCode >= 400) {
      throw JcApiError.fromJson(
        jsonDecode(response.body) as Map<String, dynamic>,
      );
    }

    return parse(jsonDecode(response.body));
  }

  Future<bool> refresh() => _refresh();

  Future<bool> _refresh() async {
    final refreshToken = await tokenStore.readRefreshToken();
    if (refreshToken == null) {
      clearAccessToken();
      return false;
    }
    try {
      final data = await _request<Map<String, dynamic>>(
        'POST',
        '/auth/refresh',
        body: {'refresh_token': refreshToken},
        parse: (json) => json as Map<String, dynamic>,
        includeAuth: false,
      );
      _accessToken = data['access_token'] as String;
      await tokenStore.writeRefreshToken(
        data['refresh_token'] as String,
      );
      return true;
    } catch (_) {
      onUnauthenticated?.call();
      await logout();
      return false;
    }
  }

  Future<void> logout() async {
    try {
      await post<void>('/auth/logout', parse: (_) {});
    } catch (_) {}
    clearAccessToken();
    await tokenStore.deleteRefreshToken();
  }
}
