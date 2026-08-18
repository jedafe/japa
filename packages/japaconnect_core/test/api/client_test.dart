import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_core/api/api_error.dart';

void main() {
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
  });
}
