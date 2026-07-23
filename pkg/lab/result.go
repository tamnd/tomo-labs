package lab

import "github.com/tamnd/tomo-labs/pkg/result"

// The run-outcome model lives in pkg/result, the single definition both the
// writer here and the reader in pkg/publish share. It cannot live in either of
// them: pkg/lab imports pkg/publish to publish a finished run, so publish cannot
// import lab back, and a shared leaf package is the standard way two sides of one
// on-disk contract agree on its shape without copying it. These aliases let the
// run loop keep writing lab.Result while that shape is defined once.
type (
	Result        = result.Result
	Tokens        = result.Tokens
	Latency       = result.Latency
	RateLimit     = result.RateLimit
	StreamFail    = result.StreamFail
	Orchestration = result.Orchestration
)
