import 'package:http/http.dart' as http;

class Api {
  static const baseUrl = 'http://localhost:8080';

  static http.Client client = http.Client();

  static Future<String> fetchHello() async {
    final res = await client.get(Uri.parse('$baseUrl/'));
    if (res.statusCode != 200) {
      throw Exception('HTTP ${res.statusCode}');
    }
    return res.body.trim();
  }
}