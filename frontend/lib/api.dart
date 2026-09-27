import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

/// A failed API call, carrying the status so the UI can react to *why* it
/// failed rather than showing one generic error for every case.
///
/// The distinction matters: 401 means send the user back to login, 402 means the
/// cart needs topping up, 403 means the screen is not theirs to see, and 500
/// means try again. Collapsing these into "something went wrong" would make the
/// app unhelpful exactly when it needs to be specific.
class ApiException implements Exception {
  final int statusCode;
  final String message;

  ApiException(this.statusCode, this.message);

  /// The session is gone or was never valid.
  bool get isUnauthorized => statusCode == 401;

  /// Authenticated, but not allowed to do this.
  bool get isForbidden => statusCode == 403;

  /// Not enough coins for the order.
  bool get isPaymentRequired => statusCode == 402;

  /// Our fault, so the request is worth repeating.
  bool get isRetryable => statusCode >= 500;

  @override
  String toString() => message;
}

class Api {
  static const baseUrl = 'http://localhost:8080';

  static http.Client client = http.Client();

  /// The bearer token, or null when signed out. Every authenticated call reads
  /// this, so signing in and out is a single assignment rather than a token
  /// threaded through every call site.
  static String? token;

  static Map<String, String> _headers() => {
    'Content-Type': 'application/json',
    if (token != null) 'Authorization': 'Bearer $token',
  };

  /// Turns a non-2xx response into an ApiException carrying the server's own
  /// message, which is written to be user-facing.
  static ApiException _error(http.Response res) {
    String message = 'HTTP ${res.statusCode}';
    try {
      final decoded = jsonDecode(res.body);
      if (decoded is Map<String, dynamic> && decoded['error'] is String) {
        message = decoded['error'] as String;
      }
    } on FormatException {
      // Not JSON, for example an HTML error page from a proxy in front of the
      // server. The status code is still the useful part.
    }
    return ApiException(res.statusCode, message);
  }

  static Future<dynamic> _send(Future<http.Response> Function() run) async {
    final res = await run();
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw _error(res);
    }
    if (res.body.isEmpty) {
      return null;
    }
    return jsonDecode(res.body);
  }

  static Future<List<Dish>> fetchMenu() async {
    final data = await _send(
      () => client.get(
        Uri.parse('$baseUrl/api/menu'),
        headers: {'Content-Type': 'application/json'},
      ),
    );
    return (data as List<dynamic>)
        .map((e) => Dish.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// Signs in and returns the user. Throws ApiException(401) with a generic
  /// message for both an unknown email and a wrong password, because the server
  /// does not distinguish them and neither should the app.
  static Future<User> login(String email, String password) async {
    final data = await _send(
      () => client.post(
        Uri.parse('$baseUrl/api/auth/login'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'email': email, 'password': password}),
      ),
    );
    final map = data as Map<String, dynamic>;
    token = map['token'] as String;
    return User.fromJson(map);
  }

  static Future<User> me() async {
    final data = await _send(
      () => client.get(Uri.parse('$baseUrl/api/me'), headers: _headers()),
    );
    return User.fromJson(data as Map<String, dynamic>);
  }

  static Future<void> logout() async {
    token = null;
  }

  /// The orders visible to the signed-in user. The server decides what that is:
  /// a student only ever receives their own, so there is no userId parameter to
  /// get wrong.
  static Future<List<Order>> fetchOrders({String? status}) async {
    var uri = Uri.parse('$baseUrl/api/orders');
    if (status != null) {
      uri = uri.replace(queryParameters: {'status': status});
    }
    final data = await _send(() => client.get(uri, headers: _headers()));
    return (data as List<dynamic>)
        .map((e) => Order.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// Places an order. The cart carries only dish ids and quantities; the server
  /// looks up the prices, so a tampered client cannot name its own price.
  static Future<Order> placeOrder(Map<int, int> cart) async {
    if (cart.isEmpty) {
      // Checked here as well as on the server: an empty cart is a dead end, and
      // spending a request to learn that wastes a round trip and would let a
      // bug that empties the cart look like a server problem.
      throw ApiException(400, 'Cart is empty');
    }
    final items = cart.entries
        .map((e) => {'menu_item_id': e.key, 'qty': e.value})
        .toList();
    final data = await _send(
      () => client.post(
        Uri.parse('$baseUrl/api/orders'),
        headers: _headers(),
        body: jsonEncode({'items': items}),
      ),
    );
    return Order.fromJson(data as Map<String, dynamic>);
  }

  /// Moves an order along the kitchen queue. Canteen and admin only.
  static Future<Order> setOrderStatus(int orderId, String status) async {
    final data = await _send(
      () => client.patch(
        Uri.parse('$baseUrl/api/orders/$orderId/status'),
        headers: _headers(),
        body: jsonEncode({'status': status}),
      ),
    );
    return Order.fromJson(data as Map<String, dynamic>);
  }

  static Future<List<CoinEntry>> fetchCoinHistory(int userId) async {
    final data = await _send(
      () => client.get(
        Uri.parse('$baseUrl/api/users/$userId/coins'),
        headers: _headers(),
      ),
    );
    return (data as List<dynamic>)
        .map((e) => CoinEntry.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// Admin only: credits a user for cash they paid at the counter.
  static Future<User> exchangeCoins(
    int userId,
    int amount,
    String reason,
  ) async {
    final data = await _send(
      () => client.post(
        Uri.parse('$baseUrl/api/admin/users/$userId/coins'),
        headers: _headers(),
        body: jsonEncode({'amount': amount, 'reason': reason}),
      ),
    );
    return User.fromJson(data as Map<String, dynamic>);
  }
}
