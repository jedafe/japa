//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class TrendingApi {
  TrendingApi([ApiClient? apiClient])
      : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// Trending content
  ///
  /// Retrieve trending content snapshots scored by recent engagement.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<Response> publicTrendingContentGetWithHttpInfo({
    int? limit,
    String? window,
    String? geo,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/trending/content';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (window != null) {
      queryParams.addAll(_queryParams('', 'window', window));
    }
    if (geo != null) {
      queryParams.addAll(_queryParams('', 'geo', geo));
    }

    const contentTypes = <String>[];

    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
    );
  }

  /// Trending content
  ///
  /// Retrieve trending content snapshots scored by recent engagement.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<PublicTrendingContentGet200Response?> publicTrendingContentGet({
    int? limit,
    String? window,
    String? geo,
  }) async {
    final response = await publicTrendingContentGetWithHttpInfo(
      limit: limit,
      window: window,
      geo: geo,
    );
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty &&
        response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(
        await _decodeBodyBytes(response),
        'PublicTrendingContentGet200Response',
      ) as PublicTrendingContentGet200Response;
    }
    return null;
  }

  /// Trending spaces
  ///
  /// Retrieve trending space snapshots scored by recent member activity.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<Response> publicTrendingSpacesGetWithHttpInfo({
    int? limit,
    String? window,
    String? geo,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/trending/spaces';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (window != null) {
      queryParams.addAll(_queryParams('', 'window', window));
    }
    if (geo != null) {
      queryParams.addAll(_queryParams('', 'geo', geo));
    }

    const contentTypes = <String>[];

    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
    );
  }

  /// Trending spaces
  ///
  /// Retrieve trending space snapshots scored by recent member activity.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<PublicTrendingContentGet200Response?> publicTrendingSpacesGet({
    int? limit,
    String? window,
    String? geo,
  }) async {
    final response = await publicTrendingSpacesGetWithHttpInfo(
      limit: limit,
      window: window,
      geo: geo,
    );
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty &&
        response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(
        await _decodeBodyBytes(response),
        'PublicTrendingContentGet200Response',
      ) as PublicTrendingContentGet200Response;
    }
    return null;
  }

  /// Trending tags
  ///
  /// Retrieve trending tag snapshots scored by recent follower growth.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<Response> publicTrendingTagsGetWithHttpInfo({
    int? limit,
    String? window,
    String? geo,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/trending/tags';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (window != null) {
      queryParams.addAll(_queryParams('', 'window', window));
    }
    if (geo != null) {
      queryParams.addAll(_queryParams('', 'geo', geo));
    }

    const contentTypes = <String>[];

    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
    );
  }

  /// Trending tags
  ///
  /// Retrieve trending tag snapshots scored by recent follower growth.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<PublicTrendingContentGet200Response?> publicTrendingTagsGet({
    int? limit,
    String? window,
    String? geo,
  }) async {
    final response = await publicTrendingTagsGetWithHttpInfo(
      limit: limit,
      window: window,
      geo: geo,
    );
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty &&
        response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(
        await _decodeBodyBytes(response),
        'PublicTrendingContentGet200Response',
      ) as PublicTrendingContentGet200Response;
    }
    return null;
  }

  /// Trending users
  ///
  /// Retrieve trending user snapshots scored by recent engagement.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<Response> publicTrendingUsersGetWithHttpInfo({
    int? limit,
    String? window,
    String? geo,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/trending/users';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (window != null) {
      queryParams.addAll(_queryParams('', 'window', window));
    }
    if (geo != null) {
      queryParams.addAll(_queryParams('', 'geo', geo));
    }

    const contentTypes = <String>[];

    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
    );
  }

  /// Trending users
  ///
  /// Retrieve trending user snapshots scored by recent engagement.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [String] window:
  ///   Time window for the trending snapshot.
  ///
  /// * [String] geo:
  ///   Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.
  Future<PublicTrendingContentGet200Response?> publicTrendingUsersGet({
    int? limit,
    String? window,
    String? geo,
  }) async {
    final response = await publicTrendingUsersGetWithHttpInfo(
      limit: limit,
      window: window,
      geo: geo,
    );
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty &&
        response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(
        await _decodeBodyBytes(response),
        'PublicTrendingContentGet200Response',
      ) as PublicTrendingContentGet200Response;
    }
    return null;
  }
}
