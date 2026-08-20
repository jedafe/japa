//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

library openapi.api;

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:collection/collection.dart';
import 'package:http/http.dart';
import 'package:intl/intl.dart';
import 'package:meta/meta.dart';

part 'api_client.dart';
part 'api_helper.dart';
part 'api_exception.dart';
part 'auth/authentication.dart';
part 'auth/api_key_auth.dart';
part 'auth/oauth.dart';
part 'auth/http_basic_auth.dart';
part 'auth/http_bearer_auth.dart';

part 'api/content_api.dart';
part 'api/search_api.dart';
part 'api/spaces_api.dart';
part 'api/tags_api.dart';
part 'api/trending_api.dart';
part 'api/users_api.dart';

part 'model/comment.dart';
part 'model/content.dart';
part 'model/error.dart';
part 'model/error_error.dart';
part 'model/public_content_get200_response.dart';
part 'model/public_content_id_comments_get200_response.dart';
part 'model/public_content_id_get200_response.dart';
part 'model/public_me_get200_response.dart';
part 'model/public_spaces_get200_response.dart';
part 'model/public_spaces_slug_get200_response.dart';
part 'model/public_tags_get200_response.dart';
part 'model/public_tags_slug_get200_response.dart';
part 'model/public_trending_content_get200_response.dart';
part 'model/public_users_get200_response.dart';
part 'model/space.dart';
part 'model/tag.dart';
part 'model/trending_item.dart';
part 'model/user.dart';

/// An [ApiClient] instance that uses the default values obtained from
/// the OpenAPI specification file.
var defaultApiClient = ApiClient();

const _delimiters = {'csv': ',', 'ssv': ' ', 'tsv': '\t', 'pipes': '|'};
const _dateEpochMarker = 'epoch';
const _deepEquality = DeepCollectionEquality();
final _dateFormatter = DateFormat('yyyy-MM-dd');
final _regList = RegExp(r'^List<(.*)>$');
final _regSet = RegExp(r'^Set<(.*)>$');
final _regMap = RegExp(r'^Map<String,(.*)>$');

bool _isEpochMarker(String? pattern) =>
    pattern == _dateEpochMarker || pattern == '/$_dateEpochMarker/';
