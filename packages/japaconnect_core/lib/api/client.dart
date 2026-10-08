import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:japaconnect_core/auth/token_store.dart';
import 'package:japaconnect_core/auth/device_id.dart';
import 'package:japaconnect_core/auth/auth_state.dart';
import 'api_error.dart';
import 'package:japaconnect_core/models/jce.dart';

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

  /// Surface identifier sent on every backend call (X-JC-Surface). The
  /// backend resolves feature flags and geo rules per surface — valid
  /// values are defined in internal/features/eval.go (ValidSurface).
  final String surface;
  String? _accessToken;
  final http.Client _http = http.Client();

  JcApiClient({
    required this.baseUrl,
    this.wsUrl = '',
    required this.tokenStore,
    required this.deviceId,
    this.onUnauthenticated,
    this.surface = 'mobile',
  });

  String get effectiveWsUrl {
    if (wsUrl.isNotEmpty) return wsUrl;
    // baseUrl already includes the API prefix (e.g. .../api/v1) — the WS
    // endpoint lives at <base>/ws.
    final base = baseUrl
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://')
        .replaceFirst(RegExp(r'/+$'), '');
    return '$base/ws';
  }

  String? get accessToken => _accessToken;

  void setAccessToken(String token) => _accessToken = token;
  void clearAccessToken() => _accessToken = null;

  Future<Map<String, String>> _buildHeaders({
    bool includeAuth = true,
  }) async {
    final headers = <String, String>{
      'X-Device-Id': await deviceId.value,
      'X-JC-Surface': surface,
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

  Future<dynamic> _rawRequest(
    String method,
    String path, {
    Map<String, dynamic>? query,
    Map<String, dynamic>? body,
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

    final responseBody = response.body;
    dynamic decoded;
    if (responseBody.isNotEmpty) {
      decoded = jsonDecode(responseBody);
    }

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
      return _rawRequest(
        method,
        path,
        query: query,
        body: body,
        includeAuth: includeAuth,
        isRetry: true,
      );
    }

    if (response.statusCode >= 400) {
      throw JcApiError.fromJson(
        (decoded ?? {}) as Map<String, dynamic>,
        fallbackStatus: response.statusCode,
      );
    }

    return decoded;
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
    final decoded = await _rawRequest(
      method,
      path,
      query: query,
      body: body,
      includeAuth: includeAuth,
      isRetry: isRetry,
    );

    // JEOS API responses wrap the payload in a top-level `data` key.
    if (decoded is Map<String, dynamic> && decoded.containsKey('data')) {
      return parse(decoded['data']);
    }
    return parse(decoded);
  }

  Future<PaginatedList<T>> getList<T>(
    String path, {
    Map<String, dynamic>? query,
    required T Function(dynamic) parse,
  }) async {
    final decoded = await _rawRequest('GET', path, query: query);
    final data = (decoded as Map<String, dynamic>?)?['data'];
    final metaJson = (decoded as Map<String, dynamic>?)?['meta'];
    final List<T> items;
    if (data is List) {
      items = data.map(parse).cast<T>().toList();
    } else {
      items = [];
    }
    final meta = metaJson == null
        ? null
        : ListMeta.fromJson(metaJson as Map<String, dynamic>);
    return PaginatedList<T>(items: items, meta: meta);
  }

  Stream<Map<String, dynamic>> streamSse(
    String path, {
    String method = 'POST',
    Map<String, dynamic>? body,
  }) async* {
    final headers = await _buildHeaders();
    headers['Content-Type'] = 'application/json';
    headers['Accept'] = 'text/event-stream';

    final uri = Uri.parse('$baseUrl$path');
    final request = http.Request(method, uri);
    request.headers.addAll(headers);
    if (body != null) request.body = jsonEncode(body);

    final streamedResponse = await _http.send(request);

    if (streamedResponse.statusCode >= 400) {
      final response = await http.Response.fromStream(streamedResponse);
      dynamic decoded;
      if (response.body.isNotEmpty) decoded = jsonDecode(response.body);
      throw JcApiError.fromJson(
        (decoded ?? {}) as Map<String, dynamic>,
        fallbackStatus: streamedResponse.statusCode,
      );
    }

    final stream = streamedResponse.stream
        .transform(utf8.decoder)
        .transform(const LineSplitter());

    await for (final line in stream) {
      final trimmed = line.trim();
      if (trimmed.isEmpty) continue;
      if (trimmed.startsWith('data:')) {
        final text = trimmed.substring(5).trim();
        if (text.isEmpty) continue;
        try {
          final json = jsonDecode(text) as Map<String, dynamic>;
          yield json;
        } catch (_) {
          yield {'type': 'message', 'content': text};
        }
      }
    }
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
      final newRefreshToken = data['refresh_token'] as String?;
      if (newRefreshToken != null) {
        await tokenStore.writeRefreshToken(newRefreshToken);
      }
      return true;
    } catch (_) {
      onUnauthenticated?.call();
      await logout();
      return false;
    }
  }

  Future<void> logout() async {
    try {
      final refreshToken = await tokenStore.readRefreshToken();
      await post<void>(
        '/auth/logout',
        body: refreshToken != null ? {'refresh_token': refreshToken} : null,
        parse: (_) {},
      );
    } catch (_) {}
    clearAccessToken();
    await tokenStore.deleteRefreshToken();
  }
}
