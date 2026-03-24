# CLAUDE.md

## Core Model

ccmux maps each Telegram chat (private, group, or topic) to a single Claude Code session running inside a tmux window. This mapping is strict and stable: one chat equals one session equals one tmux window. Telegram acts as the control plane, while Claude Code is the execution engine.

## Source of Truth

The JSONL session log is the only authoritative state. All messages, tool calls, and results must be derived from it. The terminal and hooks are secondary: they provide interaction signals but never define state. The system must always be able to reconstruct a session purely from the log.

## Execution Model

The system operates in two modes. In non-interactive mode, input is sent to tmux and output is read from JSONL. In interactive mode, the terminal is additionally observed to detect prompts and selections, which are projected into Telegram as structured UI. In both cases, JSONL remains the only output source.

## Interaction Model

Terminal interactions are abstracted into a structured model and rendered as Telegram UI. User actions are translated back into terminal input. Cursor movement, layout, and formatting details are never exposed, and interaction must not depend on terminal positioning.

## Session Lifecycle

Sessions support start, attach, recovery, and restart. Failures are expected: tmux may disappear, Claude may crash, or connectivity may drop. The system continuously reconciles state by reattaching or recreating components and resuming from the JSONL log.

## Reliability Model

The system is restart-safe and failure-tolerant. All operations must be idempotent, and outputs must be deduplicated. Components fail independently and recover without affecting overall session continuity.

## Internal Structure

The system is event-driven. JSONL produces events, the terminal provides interaction signals, and Telegram produces user input. These are coordinated through the session, with clear boundaries between parsing, interaction modeling, and transport.

## Constraints

The system must not depend on terminal formatting, hidden in-memory state, or Claude-specific UI behavior. All logic must remain deterministic, reconstructable, and resilient to change.

## Extensions

The design allows for session replay, multi-user control, meta commands, backpressure handling, stuck detection, aliases, and future provider abstraction, as long as the log-driven and deterministic model is preserved.
