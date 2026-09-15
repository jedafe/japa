//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class SpacesApi {
  SpacesApi([ApiClient? apiClient]) : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// List public spaces
  ///
  /// Retrieve a paginated list of public spaces.
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
  Future<Response> publicSpacesGetWithHttpInfo({
    int? limit,
    int? offset,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/spaces';

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

  /// List public spaces
  ///
  /// Retrieve a paginated list of public spaces.
  ///
  /// Parameters:
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<PublicSpacesGet200Response?> publicSpacesGet({
    int? limit,
    int? offset,
  }) async {
    final response = await publicSpacesGetWithHttpInfo(
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
        'PublicSpacesGet200Response',
      ) as PublicSpacesGet200Response;
    }
    return null;
  }

  /// Get space detail
  ///
  /// Retrieve a single space by its URL slug.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] slug (required):
  ///   Space slug
  Future<Response> publicSpacesSlugGetWithHttpInfo(
    String slug,
  ) async {
    // ignore: prefer_const_declarations
    final path = r'/public/spaces/{slug}'.replaceAll('{slug}', slug);

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

  /// Get space detail
  ///
  /// Retrieve a single space by its URL slug.
  ///
  /// Parameters:
  ///
  /// * [String] slug (required):
  ///   Space slug
  Future<PublicSpacesSlugGet200Response?> publicSpacesSlugGet(
    String slug,
  ) async {
    final response = await publicSpacesSlugGetWithHttpInfo(
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
        'PublicSpacesSlugGet200Response',
      ) as PublicSpacesSlugGet200Response;
    }
    return null;
  }
}
