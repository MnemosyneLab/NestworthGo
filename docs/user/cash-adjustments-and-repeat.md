# Review a cash adjustment or repeat an activity

In an account with cash and holdings, choose **Reconcile balance**, select the
currency and enter the balance you want. You may add a reason. Choose **Preview**
to see the current balance, resulting balance, signed adjustment and effective
time in the History timezone (UTC before History starts). When standard currency
formatting rounds an amount, the review also shows its exact value, so a small
nonzero adjustment remains visible. Preview saves nothing. Choose **Confirm** to record the adjustment.
Before History starts, **Add cash balance** uses the same review to establish a
baseline; a reason is available only for adjustments after History starts.

In History, open an ordinary single-account cash record's **Actions** menu or
**Details**, then choose **Repeat as new draft**. Income reasons supported are
income, interest, contribution, gift and other. Spending reasons supported are
expense, fee, tax, interest and other. The reason stays the same by default and
is editable, along with account, amount, currency and note. The date/time
defaults to now. Choose **Preview** and **Confirm**; opening the draft posts
nothing. Interest retains its existing money classification.

Dividend, holding-linked fee/tax, product, transfer, trade, reconciliation,
correction, reversal and already undone records cannot be repeated as ordinary
cash. Details explains when Repeat is unavailable; a fee or tax attributed to a
holding has its own explanation rather than being described as a dividend.
Eligibility is checked from the full ledger, even when the reversing activity
is outside your current History filter or page, and checked again at preview
and confirmation.

**Back to draft** preserves your input. Editing any field requires another
preview. Closing the sheet discards the draft; reopening starts afresh. If a
balance or account changes, or a preview expires, review a fresh preview before
confirming. If a later cash observation would override the reviewed time, check
the system clock and cash history, then preview again. The app keeps the reviewed
financial time unchanged. After an uncertain confirmation error, use **Retry same confirmation**
so it can return its original result without another record. If the plan is
expired or unavailable after an uncertain confirmation, inspect History before
creating a fresh preview; its confirmation would be a new operation. If the app was
restarted, inspect History before repeating an operation with an unknown outcome.

When confirmation is definitively rejected, the retained confirmation is
cleared: review a fresh preview if the source remains eligible. If the source
was undone or otherwise refused, close the draft and select another activity.

History details show the sign of value increases and decreases, including on
exact decimal values that round to zero in the currency display.
