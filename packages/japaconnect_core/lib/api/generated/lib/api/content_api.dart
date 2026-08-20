//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class ContentApi {
  ContentApi([ApiClient? apiClient])
      : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// List published content
  ///
  /// Retrieve a paginated list of published, public content within the API key's tenant. Drafts, scheduled, and private content are never exposed.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<Response> publicContentGetWithHttpInfo({
    int? limit,
    int? offset,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/content';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (offset != null) {
      queryParams.addAll(_queryParams('', 'offset', offset));
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

  /// List published content
  ///
  /// Retrieve a paginated list of published, public content within the API key's tenant. Drafts, scheduled, and private content are never exposed.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<PublicContentGet200Response?> publicContentGet({
    int? limit,
    int? offset,
  }) async {
    final response = await publicContentGetWithHttpInfo(
      limit: limit,
      offset: offset,
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
        'PublicContentGet200Response',
      ) as PublicContentGet200Response;
    }
    return null;
  }

  /// List content comments
  ///
  /// Retrieve top-level published comments for a content item, ordered by pinned then vote score.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   Content ID
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<Response> publicContentIdCommentsGetWithHttpInfo(
    String id, {
    int? limit,
    int? offset,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/content/{id}/comments'.replaceAll('{id}', id);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (limit != null) {
      queryParams.addAll(_queryParams('', 'limit', limit));
    }
    if (offset != null) {
      queryParams.addAll(_queryParams('', 'offset', offset));
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

  /// List content comments
  ///
  /// Retrieve top-level published comments for a content item, ordered by pinned then vote score.
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   Content ID
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<PublicContentIdCommentsGet200Response?> publicContentIdCommentsGet(
    String id, {
    int? limit,
    int? offset,
  }) async {
    final response = await publicContentIdCommentsGetWithHttpInfo(
      id,
      limit: limit,
      offset: offset,
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
        'PublicContentIdCommentsGet200Response',
      ) as PublicContentIdCommentsGet200Response;
    }
    return null;
  }

  /// Get content detail
  ///
  /// Retrieve a single published content item by its unique identifier.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   Content ID
  Future<Response> publicContentIdGetWithHttpInfo(
    String id,
  ) async {
    // ignore: prefer_const_declarations
    final path = r'/public/content/{id}'.replaceAll('{id}', id);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

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

  /// Get content detail
  ///
  /// Retrieve a single published content item by its unique identifier.
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   Content ID
  Future<PublicContentIdGet200Response?> publicContentIdGet(
    String id,
  ) async {
    final response = await publicContentIdGetWithHttpInfo(
      id,
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
        'PublicContentIdGet200Response',
      ) as PublicContentIdGet200Response;
    }
    return null;
  }
}
