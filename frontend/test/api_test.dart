import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:frontend/core/api.dart';
import 'package:frontend/core/models.dart';

void main() {
  tearDown(() {
    Api.token = null;
    Api.client = http.Client();
  });

  group('login', () {
    const ok =
        '{"id":17,"name":"Ravi","email":"ravi@x.test","role":"student",'
        '"coin_balance":110,"token":"jwt-here"}';

    test('stores the token and returns the user', () async {
      Api.client = MockClient((r) async => http.Response(ok, 200));
      final user = await Api.login('ravi@x.test', 'pw');

      expect(user.name, 'Ravi');
      expect(user.role, Role.student);
      expect(user.coinBalance, 110);
      // The token is kept so later calls can send it without threading it around.
      expect(Api.token, 'jwt-here');
    });

    test('sends the credentials in the body', () async {
      String? body;
      Api.client = MockClient((r) async {
        body = r.body;
        return http.Response(ok, 200);
      });
      await Api.login('ravi@x.test', 'hunter2');

      expect(body, contains('ravi@x.test'));
      expect(body, contains('hunter2'));
      expect(body, isNot(contains('token')));
    });

    test(
      'a wrong password is unauthorized and the server message is kept',
      () async {
        Api.client = MockClient(
          (r) async =>
              http.Response('{"error":"invalid email or password"}', 401),
        );

        await expectLater(
          Api.login('ravi@x.test', 'wrong'),
          throwsA(
            isA<ApiException>()
                .having((e) => e.statusCode, 'statusCode', 401)
                .having(
                  (e) => e.message,
                  'message',
                  'invalid email or password',
                ),
          ),
        );
        // A failed sign-in must not leave a stale token behind.
        expect(Api.token, isNull);
      },
    );
  });

  group('ApiException classification', () {
    // Each of these drives a different response in the UI, so the mapping from
    // status to meaning is worth pinning.
    test('401 means send the user back to login', () async {
      Api.client = MockClient(
        (r) async => http.Response('{"error":"nope"}', 401),
      );
      await expectLater(
        Api.me(),
        throwsA(
          predicate(
            (e) => e is ApiException && e.isUnauthorized && !e.isForbidden,
          ),
        ),
      );
    });

    test('403 means authenticated but not allowed', () async {
      Api.client = MockClient(
        (r) async => http.Response('{"error":"nope"}', 403),
      );
      await expectLater(
        Api.fetchOrders(),
        throwsA(
          predicate(
            (e) => e is ApiException && e.isForbidden && !e.isUnauthorized,
          ),
        ),
      );
    });

    test('402 means not enough coins, so the cart can be topped up', () async {
      Api.client = MockClient(
        (r) async => http.Response('{"error":"not enough coins"}', 402),
      );
      await expectLater(
        Api.placeOrder({1: 1}),
        throwsA(predicate((e) => e is ApiException && e.isPaymentRequired)),
      );
    });

    test('5xx is retryable, 4xx is not', () async {
      Api.client = MockClient(
        (r) async => http.Response('{"error":"boom"}', 500),
      );
      await expectLater(
        Api.me(),
        throwsA(
          predicate(
            (e) => e is ApiException && e.isRetryable && !e.isUnauthorized,
          ),
        ),
      );

      Api.client = MockClient(
        (r) async => http.Response('{"error":"boom"}', 400),
      );
      await expectLater(
        Api.me(),
        throwsA(predicate((e) => e is ApiException && !e.isRetryable)),
      );
    });

    test('a non-JSON error body falls back to the status code', () async {
      // A proxy in front of the server returns HTML, not our JSON.
      Api.client = MockClient(
        (r) async => http.Response(
          '<html>502</html>',
          502,
          headers: {'content-type': 'text/html'},
        ),
      );
      await expectLater(
        Api.me(),
        throwsA(predicate((e) => e is ApiException && e.statusCode == 502)),
      );
    });
  });

  group('auth header', () {
    test('is sent once a token exists', () async {
      Api.token = 'jwt-here';
      String? auth;
      Api.client = MockClient((r) async {
        auth = r.headers['Authorization'];
        return http.Response('[]', 200);
      });
      await Api.fetchOrders();
      expect(auth, 'Bearer jwt-here');
    });

    test('is omitted when signed out', () async {
      String? auth;
      Api.client = MockClient((r) async {
        auth = r.headers['Authorization'];
        return http.Response('[]', 200);
      });
      await Api.fetchOrders();
      expect(auth, isNull);
    });

    test('logout clears the token', () async {
      Api.token = 'jwt-here';
      await Api.logout();
      expect(Api.token, isNull);
    });
  });

  group('placeOrder', () {
    test('sends only ids and quantities, never prices', () async {
      // The server must be the one to price the cart. A client that could name
      // a price would be a client that could set its own cost.
      String? body;
      Api.token = 'jwt';
      Api.client = MockClient((r) async {
        body = r.body;
        return http.Response(
          '{"id":5,"user_id":17,"customer":"Ravi","total":40,"status":"pending",'
          '"items":[],"created_at":"","updated_at":""}',
          201,
        );
      });
      await Api.placeOrder({34: 2, 35: 1});

      expect(body, contains('"menu_item_id":34'));
      expect(body, contains('"qty":2'));
      expect(body, isNot(contains('price')));
    });

    test('an empty cart is not sent', () async {
      Api.token = 'jwt';
      bool called = false;
      Api.client = MockClient((r) async {
        called = true;
        return http.Response('{}', 201);
      });
      // The server rejects an empty cart, but the client refuses to spend a
      // request on it and never invents a line to make the request look valid.
      await expectLater(
        Api.placeOrder({}),
        throwsA(predicate((e) => e is ApiException && e.statusCode == 400)),
      );
      expect(called, isFalse);
    });
  });

  group('parsing', () {
    test('Order keeps the snapshot name and totals the server sent', () {
      final order = Order.fromJson({
        'id': 5,
        'user_id': 17,
        'customer': 'Ravi',
        'total': 40,
        'status': 'pending',
        'items': [
          {
            'menu_item_id': 34,
            'name': 'Masala Chai',
            'qty': 2,
            'unit_price': 10,
            'line_total': 20,
          },
        ],
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': '2026-01-01T00:00:00Z',
      });

      expect(order.total, 40);
      expect(order.items.single.name, 'Masala Chai');
      expect(order.items.single.lineTotal, 20);
    });

    test('a coin entry keeps the sign that says which way coins moved', () {
      final spent = CoinEntry.fromJson({
        'id': 9,
        'amount': -40,
        'kind': 'order_payment',
        'reason': 'order #5',
        'actor_id': null,
        'order_id': 5,
        'created_at': '2026-01-01T00:00:00Z',
      });
      final earned = CoinEntry.fromJson({
        'id': 10,
        'amount': 40,
        'kind': 'canteen_revenue',
        'reason': 'order #5',
        'actor_id': null,
        'order_id': 5,
        'created_at': '2026-01-01T00:00:00Z',
      });

      expect(spent.amount, -40);
      expect(spent.orderId, 5);
      expect(earned.amount, 40);
      expect(spent.kindLabel, 'Order paid');
    });

    test('dish prices parse as whole coins', () {
      final dish = Dish.fromJson({
        'id': 1,
        'name': 'Masala Dosa',
        'category': 'Breakfast',
        'price': 50,
        'description': '',
        'available': true,
      });
      expect(dish.price, 50);
    });
  });

  group('role helpers', () {
    test('only the canteen and admin see every order', () {
      expect(Role.seesAllOrders(Role.admin), isTrue);
      expect(Role.seesAllOrders(Role.canteenManagement), isTrue);
      expect(Role.seesAllOrders(Role.student), isFalse);
      expect(Role.seesAllOrders(Role.staff), isFalse);
    });
  });
}
