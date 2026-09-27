import 'package:flutter/material.dart';

import 'api.dart';
import 'failure_text.dart';
import 'session.dart';

/// Shows [message] in a snackbar, replacing whatever is on screen.
///
/// One queue, one message. Two snackbars at once is how a user ends up
/// reading a success notice over the error that replaced it, or tapping a
/// button behind a message about a different button.
void showMessage(BuildContext context, String message) {
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(message)));
}

/// Runs [action] and reports a failure the way every screen should.
///
/// Returns what [action] produced, or null if it did not succeed. Every call
/// site here returns something non-null on success, so null means failure and
/// needs no separate result type.
///
/// A 401 signs the session out rather than showing a message, because the token
/// is gone and every other screen would fail too. Leaving someone tapping
/// buttons that cannot work is worse than an honest return to sign-in, and a
/// message telling them to retry would be advice that has already failed.
///
/// The caller still owns its own busy flag and any work it wants to do on
/// success; only the failure handling is shared here.
Future<T?> runMutation<T>(
  BuildContext context,
  Future<T> Function() action,
) async {
  try {
    return await action();
  } on ApiException catch (e) {
    // Nothing is shown or looked up once the widget is gone: the context would
    // be defunct, and a message about a screen that no longer exists is noise.
    if (!context.mounted) return null;
    if (needsSignOut(e)) {
      await SessionScope.read(context).signOut();
      return null;
    }
    showMessage(context, failureMessage(e));
    return null;
  } on Exception catch (e) {
    if (!context.mounted) return null;
    showMessage(context, failureMessage(ApiException.networkFailure(e)));
    return null;
  }
}
