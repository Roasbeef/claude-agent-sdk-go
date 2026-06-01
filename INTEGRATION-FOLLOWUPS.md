# Integration Followups

- Backfill `TestIntegrationStopHookBackgroundTasks` when the CLI test fixture can deterministically create a long-running background task.
- Backfill `TestIntegrationStopHookSessionCrons` when the CLI test fixture can deterministically create a session cron.
- Backfill `TestIntegrationAssistantMessageSubagentFields` when the CLI test fixture can deterministically run a subagent task end-to-end.
- Backfill `TestIntegrationAssistantMessageError` when the CLI test fixture can deterministically force an upstream API failure into an `assistant` event.
