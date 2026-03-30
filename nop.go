package sense

import (
	"context"
	"encoding/json"
)

// Nop returns a no-op session that makes no API calls and returns zero values.
// Use it when sense is optional — e.g., no API key, test environments, or CI.
//
//	s := sense.Nop()
//	s.Extract("text", &dst).Run() // returns immediately, dst is unchanged
//
// Options are accepted so callers can configure the session if needed:
//
//	s := sense.Nop(sense.WithModel("claude-haiku-4-5-20251001"))
func Nop(opts ...Option) *Session {
	cfg := &sessionConfig{}
	for _, o := range opts {
		o(cfg)
	}
	applyDefaults(cfg)
	return &Session{
		client:        &nopCaller{},
		model:         cfg.model,
		timeout:       cfg.timeout,
		maxRetries:    cfg.maxRetries,
		context:       cfg.context,
		minConfidence: cfg.minConfidence,
		logger:        cfg.logger,
		hook:          cfg.hook,
	}
}

type nopCaller struct{}

func (n *nopCaller) call(_ context.Context, _ *callRequest) (json.RawMessage, *Usage, error) {
	return json.RawMessage("{}"), &Usage{}, nil
}
