import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

class Api {
  static const baseUrl = 'http://localhost:8080';

  static http.Client client = http.Client();

  static Future<List<Dish>> fetchMenu() async {
    final res = await client.get(Uri.parse('$baseUrl/api/menu'));
    if (res.statusCode != 200) {
      throw Exception('HTTP ${res.statusCode}');
    }
    final data = jsonDecode(res.body) as List<dynamic>;
    return data
        .map((e) => Dish.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}