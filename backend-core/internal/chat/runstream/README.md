# Chat runstream

## Purpose
Carries a chat run's events from the worker that executes it to the API replica that streams them to the browser, over
one Redis Stream per run (`run:{runId}`). The database stays the authority: the stream is a delivery channel that can
be incomplete, and `RelayEnd` turns every gap into a truthful ending (amendment W4). It does not run models, encode
the UI Message Stream (that is `uistream`) or decide what a run is (that is `conversation`).

## Entry points
- `Writer.Append` / `Writer.WriteTerminal` (`writer.go`): the worker's side. One `XADD` per event with the fields
  `event` (the JSON envelope) and `request_id`, no `MAXLEN`, and `EXPIREAT deadline_at + finish timeout +
  CORE_CHAT_RUN_STREAM_GRACE NX` in the same `MULTI`, so the first write sets the expiry and later ones leave it.
  `WriteTerminal` accepts only `Finish` or `Error`.
- `Reader.Follow` (`reader.go`): the API's side. `XREAD BLOCK` from `0`, each block bounded by
  `min(CORE_STREAM_BLOCK_TIMEOUT, time left in the window)`. An absent key means "not started yet". Returns
  `TerminalRelayed`, `NoTerminal` (window over or Redis failed, with the error) or `RowEnded` (the row is no longer
  `running`, asked through `StatusFunc` every `CORE_CHAT_RELAY_STATUS_CHECK` after both empty reads and event batches,
  and no terminal event arrived after one final block).
- `RelayEnd` (`relay.go`): the pure relay policy. `finish` only for `TerminalRelayed`; otherwise an `error` part from
  the run row: completed or cancelled → `RUN_STREAM_INCOMPLETE`, failed → the row's code, still running past
  `deadline_at + finish timeout` → `RUN_INTERRUPTED`.
- `EncodeEvent` / `DecodeEvent` / `IsTerminal` (`codec.go`): `{type, data}` envelope for all 18 `model.Event` types.
- `StreamKey` (`keys.go`).

## Dependencies
- Uses: go-redis, `chat/model`, `conversation/model` (run status), `platform/requestid`, named catalog errors.
- Used by: `chat/worker` (writer), `cmd/core` (wiring), the chat controller of 2b-5 (reader and `RelayEnd`).

## Run & test
```bash
cd backend-core
go test ./internal/chat/runstream -count=1 -p 1                       # Redis testcontainer
go test ./tests/integration -run "TestRunStream_|TestChatRequestID_" -p 1 -count=1   # with the run row
```
On Windows set `DOCKER_HOST=npipe:////./pipe/docker_engine`, `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock`
and `TESTCONTAINERS_RYUK_DISABLED=true`.

## Conventions
- The relay sends `finish` only after it relayed the worker's own terminal event. Without one it cannot prove the
  client got every delta, so the client is told to reload (`RUN_STREAM_INCOMPLETE`) and reads the stored message.
- go-redis sends `BLOCK 0` (forever) for a zero `Block`; `Follow` always clamps the block to at least 1 ms and returns
  at once when the window is already over.
- Adding a `model.Event` needs a line in `eventKinds()`; `TestCodec_RoundTripsEveryEventType` fails until it exists.
- Every entry carries the run's `request_id`, so the relay and the worker log the same id (amendment W9).

## Common failures
- `RUN_STREAM_INCOMPLETE` in a client right after a normal answer → the terminal event was written after the relay's
  last read, or Redis dropped it → the client refetches and shows the stored answer; look for the run's `request_id`.
- A stream key that never expires → the first write failed after `XADD` committed outside the `MULTI` → not possible
  with `TxPipelined`; check Redis `EXPIRETIME` support (Redis 7+).
