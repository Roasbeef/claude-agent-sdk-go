/**
 * API surface definition for @anthropic-ai/claude-agent-sdk/browser.
 *
 * This file is the source of truth for the browser export's public types.
 * It imports ONLY from agentSdkTypes.ts so the compiled .d.ts has exactly
 * one import to rewrite (./agentSdkTypes → ./sdk) for the flat package layout.
 *
 * Compiled by scripts/build-ant-sdk-typings.sh; see build-agent-sdk.sh for the
 * path rewrite and copy into the package.
 */
import type { CanUseTool, HookCallbackMatcher, HookEvent, McpServerConfig, OnElicitation, OnUserDialog, Query, SDKUserMessage } from './agentSdkTypes.js';
export type { CanUseTool, ElicitationRequest, ElicitationResult, HookCallbackMatcher, HookEvent, McpSdkServerConfigWithInstance, McpServerConfig, OnElicitation, OnUserDialog, Query, SDKAssistantMessage, SDKMessage, SDKResultMessage, SDKSystemMessage, SDKUserMessage, UserDialogRequest, UserDialogResult, } from './agentSdkTypes.js';
export { createSdkMcpServer, tool } from './agentSdkTypes.js';
export type OAuthCredential = {
    type: 'oauth';
    token: string;
};
export type AuthMessage = {
    type: 'auth';
    credential: OAuthCredential;
};
export type WebSocketOptions = {
    url: string;
    headers?: Record<string, string>;
    authMessage?: AuthMessage;
};
export type SSEOptions = {
    /** SSE read endpoint, e.g. `…/v1/code/sessions/{id}/events/stream`. */
    streamUrl: string;
    /** POST write endpoint, e.g. `…/v1/code/sessions/{id}/events`. */
    sendUrl: string;
    /**
     * The CCR session ID — required to build the `AddClientEventFromClient`
     * request body that `sendUrl` expects.
     */
    sessionId: string;
    /**
     * Headers sent on both the SSE GET and every POST. Set `Authorization` and
     * `anthropic-client-platform` here — the SDK cannot determine the host
     * surface (web / iOS / Android / desktop) itself.
     */
    headers?: Record<string, string>;
};
type BrowserQueryOptionsBase = {
    prompt: AsyncIterable<SDKUserMessage>;
    abortController?: AbortController;
    canUseTool?: CanUseTool;
    hooks?: Partial<Record<HookEvent, HookCallbackMatcher[]>>;
    mcpServers?: Record<string, McpServerConfig>;
    jsonSchema?: Record<string, unknown>;
    onElicitation?: OnElicitation;
    onUserDialog?: OnUserDialog;
    /**
     * When enabled, the remote CLI emits a `prompt_suggestion` message after
     * each turn's result. At most one per turn.
     */
    promptSuggestions?: boolean;
};
/**
 * Exactly one of `websocket` | `sse` must be provided. `sse` is the v1alpha2
 * path and is preferred for new integrations; `websocket` remains for
 * existing callers during the migration.
 */
export type BrowserQueryOptions = BrowserQueryOptionsBase & ({
    websocket: WebSocketOptions;
    sse?: never;
} | {
    sse: SSEOptions;
    websocket?: never;
});
/**
 * Create a Claude Code query in the browser over either SSE (preferred) or
 * WebSocket.
 *
 * @example
 * ```typescript
 * import { query } from '@anthropic-ai/claude-agent-sdk/browser'
 *
 * const messages = query({
 *   prompt: messageStream,
 *   sse: {
 *     streamUrl: 'https://api.example.com/v1/code/sessions/ID/events/stream',
 *     sendUrl: 'https://api.example.com/v1/code/sessions/ID/events',
 *     headers: { Authorization: `Bearer ${token}` },
 *   },
 * })
 * for await (const message of messages) {
 *   console.log(message)
 * }
 * ```
 */
export declare function query(options: BrowserQueryOptions): Query;
