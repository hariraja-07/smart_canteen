import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'api.dart';
import 'async_action.dart';
import 'async_view.dart';
import 'models.dart';
import 'session.dart';

/// The admin roster, and the coin exchange.
///
/// This screen moves other people's money, so it is deliberately awkward: the
/// amount must be a whole number, the reason is required because the ledger
/// exists to explain where coins came from, and a confirmation names the person
/// and the amount before anything is written.
class AdminPage extends StatefulWidget {
  const AdminPage({super.key});

  @override
  State<AdminPage> createState() => _AdminPageState();
}

class _AdminPageState extends State<AdminPage> {
  int _version = 0;
  bool _busy = false;

  Future<void> _exchange(User target) async {
    final result = await showDialog<_Exchange>(
      context: context,
      builder: (context) => _ExchangeDialog(target: target),
    );
    if (result == null || !mounted) return;

    setState(() => _busy = true);
    try {
      final updated = await runMutation(
        context,
        () => Api.exchangeCoins(target.id, result.amount, result.reason),
      );
      if (updated == null || !mounted) return;
      setState(() => _version++);
      showMessage(
        context,
        '${updated.name} now has ${updated.coinBalance} coins',
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final session = SessionScope.of(context);
    if (!session.isAdmin) {
      return const Center(child: Text('Not available for your role'));
    }

    return Stack(
      children: [
        AsyncView<List<User>>(
          load: Api.fetchUsers,
          refreshable: true,
          refreshToken: _version,
          isEmpty: (users) => users.isEmpty,
          emptyMessage: 'No accounts yet',
          builder: (context, users) => ListView.separated(
            padding: const EdgeInsets.all(12),
            itemCount: users.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, i) {
              final user = users[i];
              return ListTile(
                title: Text(user.name),
                subtitle: Text('${user.email}  ${user.role}'),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text('${user.coinBalance}'),
                    const SizedBox(width: 12),
                    FilledButton.tonal(
                      onPressed: _busy ? null : () => _exchange(user),
                      child: const Text('Add coins'),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
        if (_busy)
          const Positioned.fill(
            child: ColoredBox(
              color: Color(0x33000000),
              child: Center(child: CircularProgressIndicator()),
            ),
          ),
      ],
    );
  }
}

class _Exchange {
  final int amount;
  final String reason;

  const _Exchange(this.amount, this.reason);
}

class _ExchangeDialog extends StatefulWidget {
  final User target;

  const _ExchangeDialog({required this.target});

  @override
  State<_ExchangeDialog> createState() => _ExchangeDialogState();
}

class _ExchangeDialogState extends State<_ExchangeDialog> {
  final _formKey = GlobalKey<FormState>();
  final _amount = TextEditingController();
  final _reason = TextEditingController();

  @override
  void dispose() {
    _amount.dispose();
    _reason.dispose();
    super.dispose();
  }

  void _submit() {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    Navigator.of(
      context,
    ).pop(_Exchange(int.parse(_amount.text.trim()), _reason.text.trim()));
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text('Add coins for ${widget.target.name}'),
      content: Form(
        key: _formKey,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextFormField(
              controller: _amount,
              autofocus: true,
              keyboardType: TextInputType.number,
              // Digits only, because coins are whole and a decimal here would
              // be a value the server rejects.
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(
                labelText: 'Amount',
                helperText: '1 coin is 1 rupee',
                border: OutlineInputBorder(),
              ),
              validator: (v) {
                final value = int.tryParse((v ?? '').trim());
                if (value == null) return 'Enter a whole number of coins';
                if (value <= 0) return 'Must be at least 1 coin';
                return null;
              },
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _reason,
              decoration: const InputDecoration(
                labelText: 'Reason',
                helperText: 'Recorded in the ledger, so it has to say why',
                border: OutlineInputBorder(),
              ),
              validator: (v) =>
                  (v ?? '').trim().isEmpty ? 'A reason is required' : null,
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(onPressed: _submit, child: const Text('Give coins')),
      ],
    );
  }
}
