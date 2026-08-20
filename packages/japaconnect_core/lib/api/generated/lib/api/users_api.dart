//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class UsersApi {
  UsersApi([ApiClient? apiClient]) : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// Get current user profile
  ///
  /// Retrieve the authenticated user's own profile. Requires the api.read.me scope.
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> publicMeGetWithHttpInfo() async {
    // ignore: prefer_const_declarations
    final path = r'/public/me';

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

  /// Get current user profile
  ///
  /// Retrieve the authenticated user's own profile. Requires the api.read.me scope.
  Future<PublicMeGet200Response?> publicMeGet() async {
    final response = await publicMeGetWithHttpInfo();
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
        'PublicMeGet200Response',
      ) as PublicMeGet200Response;
    }
    return null;
  }

  /// List public users
  ///
  /// Retrieve a paginated list of public user profiles. PII-safe: no email, status, last_login, or group membership. Filter by ?q= (username/display_name search) and ?type= (profile_type).
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] q:
  ///   Search by username or display name
  ///
  /// * [String] type:
  ///   Filter by profile type (e.g. individual, organization)
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<Response> publicUsersGetWithHttpInfo({
    String? q,
    String? type,
    int? limit,
    int? offset,
  }) async {
    // ignore: prefer_const_declarations
    final path = r'/public/users';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }
    if (type != null) {
      queryParams.addAll(_queryParams('', 'type', type));
    }
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

  /// List public users
  ///
  /// Retrieve a paginated list of public user profiles. PII-safe: no email, status, last_login, or group membership. Filter by ?q= (username/display_name search) and ?type= (profile_type).
  ///
  /// Parameters:
  ///
  /// * [String] q:
  ///   Search by username or display name
  ///
  /// * [String] type:
  ///   Filter by profile type (e.g. individual, organization)
  ///
  /// * [int] limit:
  ///   Number of results to return.
  ///
  /// * [int] offset:
  ///   Number of results to skip for pagination.
  Future<PublicUsersGet200Response?> publicUsersGet({
    String? q,
    String? type,
    int? limit,
    int? offset,
  }) async {
    final response = await publicUsersGetWithHttpInfo(
      q: q,
      type: type,
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
        'PublicUsersGet200Response',
      ) as PublicUsersGet200Response;
    }
    return null;
  }

  /// Get user profile
  ///
  /// Retrieve a public user profile by their unique identifier.
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   User ID
  Future<Response> publicUsersIdGetWithHttpInfo(
    String id,
  ) async {
    // ignore: prefer_const_declarations
    final path = r'/public/users/{id}'.replaceAll('{id}', id);

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

  /// Get user profile
  ///
  /// Retrieve a public user profile by their unique identifier.
  ///
  /// Parameters:
  ///
  /// * [String] id (required):
  ///   User ID
  Future<PublicMeGet200Response?> publicUsersIdGet(
    String id,
  ) async {
    final response = await publicUsersIdGetWithHttpInfo(
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
        'PublicMeGet200Response',
      ) as PublicMeGet200Response;
    }
    return null;
  }
}
