class JcApiError implements Exception {
  final String code;
  final String message;
  final int status;

  JcApiError({required this.code, required this.message, required this.status});

  factory JcApiError.fromJson(Map<String, dynamic> json, {int? fallbackStatus}) {
    final error = json['error'];
    if (error is Map<String, dynamic>) {
      return JcApiError(
        code: (error['code'] as String?) ?? 'unknown_error',
        message: (error['message'] as String?) ?? 'An unexpected error occurred',
        status: (error['status'] as int?) ?? fallbackStatus ?? 500,
      );
    }
    return JcApiError(
      code: 'unknown_error',
      message: json.toString(),
      status: fallbackStatus ?? 500,
    );
  }

  @override
  String toString() => 'JcApiError($status): $code - $message';
}
