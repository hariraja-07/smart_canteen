import 'package:flutter/widgets.dart';

import 'api.dart';
import 'failure_text.dart';
import 'models.dart';

/// Who is signed in, and what that lets them reach.
///
/// Roles are never chosen by the client. The server decides what a role may
/// call, so this class only mirrors the server's answer to keep the UI from
/// showing a screen that would fail. Hiding a button is a courtesy to the user;
/// it is not the access control.
class Session extends ChangeNotifier {
  /// The client every request goes through. It lives here because the token
  /// lives there too, and the session is what decides whether there is a token.
  /// Screens reach it as `session.api`, which is why there is no separate scope
  /// for the client: everything that can talk to the API already has a session.
  final ApiClient api;

  User? _user;
  bool _busy = false;

  /// [api] is injectable so a test can supply its own transport and base URL.
  Session({ApiClient? api}) : api = api ?? ApiClient();

  User? get user => _user;
  bool get isSignedIn => _user != null;
  bool get busy => _busy;
  String get role => _user?.role ?? '';
  String get name => _user?.name ?? '';
  int get coinBalance => _user?.coinBalance ?? 0;

  /// True for the roles that can see and move the whole order queue.
  bool get canManageKitchen => Role.seesAllOrders(role);

  bool get isAdmin => role == Role.admin;

  /// Throws ApiException on bad credentials, which the login page turns into a
  /// message. The message is deliberately vague because the server will not say
  /// whether the email exists.
  Future<void> signIn(String email, String password) async {
    _busy = true;
    notifyListeners();
    try {
      _user = await api.login(email.trim(), password);
    } finally {
      _busy = false;
      notifyListeners();
    }
  }

  Future<void> signOut() async {
    await api.logout();
    _user = null;
    notifyListeners();
  }

  /// Pretends a sign-in already happened, so a test can start on a signed-in
  /// screen without a network round trip. Marked for tests only, so it cannot
  /// be mistaken for a login that the server agreed to.
  @visibleForTesting
  void debugSetUser(User user) {
    _user = user;
    notifyListeners();
  }

  /// Re-reads the balance after spending coins, so the balance in the app bar
  /// is the server's number and not a guess made before the order went through.
  Future<void> refresh() async {
    if (!isSignedIn) return;
    try {
      _user = await api.me();
    } on ApiException catch (e) {
      if (needsSignOut(e)) {
        // The token expired or was revoked. Keeping a shell the user can no
        // longer act in would strand them on screens that only fail, so sign
        // out and let them sign back in.
        _user = null;
        await api.logout();
      } else {
        // A network blip should not throw the user out of the app. The cached
        // balance may be briefly stale, which is a smaller problem than a
        // logout they did not ask for.
        return;
      }
    }
    notifyListeners();
  }
}

/// Makes the session available to the tree and rebuilds listeners when it
/// changes. Using InheritedNotifier keeps this dependency-free, which matters
/// for a project this size.
class SessionScope extends InheritedNotifier<Session> {
  const SessionScope({
    super.key,
    required Session session,
    required super.child,
  }) : super(notifier: session);

  static Session of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<SessionScope>();
    assert(scope != null, 'No SessionScope found in the widget tree');
    return scope!.notifier!;
  }

  /// Reads the session without subscribing, for callbacks that only need to act.
  static Session read(BuildContext context) {
    final scope = context.getInheritedWidgetOfExactType<SessionScope>();
    assert(scope != null, 'No SessionScope found in the widget tree');
    return scope!.notifier!;
  }
}
