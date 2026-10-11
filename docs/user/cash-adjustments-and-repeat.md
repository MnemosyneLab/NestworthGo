# Review a cash adjustment or repeat an activity

In an account with cash and holdings, choose **Reconcile balance**, select the
currency and enter the balance you want. You may add a reason. Choose **Preview**
to see the current balance, resulting balance, signed adjustment and effective
time in the History timezone (UTC before History starts). When standard currency
formatting rounds an amount, the review also shows its exact value, so a small
nonzero adjustment remains visible. Preview saves nothing. Choose **Confirm** to record the adjustment.
Before History starts, **Add cash balance** uses the same review to establish a
baseline; a reason is available only for adjustments after History starts.

In History, open an ordinary income or expense record's **Actions** menu and
choose **Repeat as new draft**. Review or edit its account, amount, currency,
note and date/time. The new time defaults to now, rather than the original date.
Then choose **Preview** and **Confirm**. Opening the draft never posts it.
Only income and expense records are supported; product operations, transfers,
trades, adjustments, corrections, reversals and records already undone do not
offer Repeat. Source eligibility is checked again when you preview and confirm;
a reversal after opening the draft prevents a new record.

**Back to draft** preserves your input. Editing any field requires another
preview. Closing the sheet discards the draft; reopening starts afresh. If a
balance or account changes, or a preview expires, review a fresh preview before
confirming. If a later cash observation would override the reviewed time, check
the system clock and cash history, then preview again. The app keeps the reviewed
financial time unchanged. After an uncertain confirmation error, retry the same confirmation
so it can return its original result without another record. If the plan is
expired or unavailable after an uncertain confirmation, inspect History before
creating a fresh preview; its confirmation would be a new operation. If the app was
restarted, inspect History before repeating an operation with an unknown outcome.
