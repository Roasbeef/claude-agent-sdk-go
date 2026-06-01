# Integration Followups

- Backfill `TestIntegrationStopHookBackgroundTasks` when the CLI test fixture can deterministically create a long-running background task.
- Backfill `TestIntegrationStopHookSessionCrons` when the CLI test fixture can deterministically create a session cron.
- Backfill `TestIntegrationAssistantMessageSubagentFields` when the CLI test fixture can deterministically run a subagent task end-to-end.
- Backfill `TestIntegrationUserMessageSubagentFields` when the CLI test fixture can deterministically run a Task-tool subagent whose tool-result user message round-trips `subagent_type` + `task_description`.
- Backfill `TestIntegrationAssistantMessageError` when the CLI test fixture can deterministically force an upstream API failure into an `assistant` event.
- Backfill `TestIntegrationResultMessageOriginTTFT` when the CLI test fixture can deterministically emit `origin` and/or `ttft_ms` on a result event.
- Backfill `TestIntegrationTaskLifecycleFields` when the CLI test fixture can deterministically run a Task-tool subagent (populates `subagent_type` on `task_started`/`task_progress`) and pause a running task (emits `task_updated` with `status:"paused"`).
