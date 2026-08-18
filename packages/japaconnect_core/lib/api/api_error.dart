class JcApiError implements Exception {
  final String code;
  final String message;
  final int status;

  JcApiError({required this.code, required this.message, required this.status});

  factory JcApiError.fromJson(Map<String, dynamic> json) {
    final error = json['error'] as Map<String, dynamic>;
    return JcApiError(
      code: error['code'] as String,
      message: error['message'] as String,
      status: error['status'] as int,
    );
  }

  @override
  String toString() => 'JcApiError($status): $code - $message';
}
