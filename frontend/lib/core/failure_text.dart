import 'api.dart';

// What a failed request should say, in one place.
//
// Four pages each spelled this out by hand and the copies drifted: cart said
// "try again in a moment" where the others said "try again", admin had no
// payment-required case at all, and kitchen carried a 409 branch whose two arms
// both returned e.message. Centralising it means a new kind of failure has to
// be understood once, not once per page.

/// True when the session is over and the user has to sign in again.
///
/// Getting this wrong in either direction is bad. Treating a 500 as terminal
/// throws a signed-in user out of the app over a blip they did not cause;
/// treating a 401 as transient leaves them sitting on screens that can only
/// fail, tapping buttons that do nothing.
bool needsSignOut(ApiException e) => e.isUnauthorized;

/// The message to show for [e].
///
/// Only the failures a user can actually act on get wording of our own.
/// Everything else passes through the server's message, which is written to be
/// read by a person and is more specific than anything invented here.
String failureMessage(ApiException e) {
  if (e.isPaymentRequired) {
    return 'Not enough coins. Visit the canteen counter to top up.';
  }
  if (e.isRetryable) {
    return 'The canteen server had a problem. Try again in a moment.';
  }
  return e.message;
}
