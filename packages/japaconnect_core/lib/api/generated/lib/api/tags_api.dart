//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class TagsApi {
  TagsApi([ApiClient? apiClient]) : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// List tags
  ///
  /// Retrieve a paginated list of content tags.
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
  Future<Response> publicTagsGetWithHttpInfo({
    int? limit,
    int? offset,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/tags';

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

  /// List tags
  ///
  /// Retrieve a paginated list of content tags.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<PublicTagsGet200Response?> publicTagsGet({
    int? limit,
    int? offset,
  }) async {
    final response = await publicTagsGetWithHttpInfo(
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
        'PublicTagsGet200Response',
      ) as PublicTagsGet200Response;
    }
    return null;
  }

  /// Get tag detail
  ///
  /// Retrieve a single tag by its URL slug, including follower and content counts.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] slug (required):
  ///   Tag slug
  Future<Response> publicTagsSlugGetWithHttpInfo(
    String slug,
  ) async {
    // ignore: prefer_const_declarations
    final path = r'/public/tags/{slug}'.replaceAll('{slug}', slug);

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

  /// Get tag detail
  ///
  /// Retrieve a single tag by its URL slug, including follower and content counts.
  ///
  /// Parameters:
  ///
  /// * [String] slug (required):
  ///   Tag slug
  Future<PublicTagsSlugGet200Response?> publicTagsSlugGet(
    String slug,
  ) async {
    final response = await publicTagsSlugGetWithHttpInfo(
      slug,
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
        'PublicTagsSlugGet200Response',
      ) as PublicTagsSlugGet200Response;
    }
    return null;
  }
}
