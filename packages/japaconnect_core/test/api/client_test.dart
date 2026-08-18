import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/api_error.dart';

void main() {
  group('JcApiError.fromJson', () {
    test('parses standard error format', () {
      final error = JcApiError.fromJson({
        'error': {
          'code': 'user_not_found',
          'message': 'No such user',
          'status': 404,
        },
      });
      expect(error.code, 'user_not_found');
      expect(error.status, 404);
      expect(error.message, 'No such user');
    });

    test('handles missing error wrapper gracefully', () {
      final error = JcApiError.fromJson(
        {'message': 'something broke'},
        fallbackStatus: 502,
      );
      expect(error.code, 'unknown_error');
      expect(error.status, 502);
      expect(error.message, contains('something broke'));
    });

    test('handles null fields inside error object', () {
      final error = JcApiError.fromJson({
        'error': <String, dynamic>{},
      });
      expect(error.code, 'unknown_error');
      expect(error.message, 'An unexpected error occurred');
      expect(error.status, 500);
    });

    test('defaults to status 500 when fallbackStatus not provided', () {
      final error = JcApiError.fromJson({'unexpected': true});
      expect(error.status, 500);
    });
  });
}
