# Browser harnesses and retained release evidence

Use the [testing guide](../testing/README.md) to choose checks and the
[real-browser workflow](../development/real-browser-acceptance.md) to launch an
isolated synthetic server. These directories retain test code and setup notes;
dated names do not make old results current. Save new outputs outside the tree
and attach exact-source evidence to the PR or CI run.

| Harness | Setup |
| --- | --- |
| Assistant component, sidebar, scrolling and semantic page reads | [Runtime](builtin-ai-readonly-2026-11-04/README.md), [sidebar](ai-sidebar-chat-2026-11-05/README.md), [scrolling](ai-live-2026-11-05/README.md), [page reads](internal-page-content-2026-11-05/README.md) |
| Real desktop/mobile page guide | [Guide](builtin-ai-page-guide-2026-11-04/README.md) |
| Mobile and desktop control boundaries | [Component and HTTP runners](atlas-remaining-2026-11-04/README.md) |
| Navigation and layout | [History](navigation-history-2026-11-04/README.md), [returns](navigation-returns-2026-11-04/README.md), [narrow desktop](desktop-narrow-2026-11-05/README.md) |
| Financial UI | [Return analysis](pr195-return-analysis/verification.md), [historical review integration fixture](pr98-pr99-main-sync-2026-10-17/README.md) |
| Assets | [Bank symbols](bank-logos/symbols/README.md), [icons](icon-coverage/README.md) |
| Historical expected-failure fixture | [Review replay](post-merge-pr101-pr102-2026-10-17/repro/README.md); not a current correctness gate |

The root README uses three historical synthetic screenshots. Test scripts and
fixture code remain alongside their setup instructions.

Retained release evidence records hashes, publication identity and explicit
acceptance limits: [v0.3.8](v0.3.8-release-package-2026-10-14.md) and
[v0.3.9](v0.3.9-release-package-2026-10-21.md). Current publication and compatibility
information belongs in the [release index](../releases/README.md).
