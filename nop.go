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
func Nop() *Session {
	return &Session{
		client: &nopCaller{},
	}
}

type nopCaller struct{}

func (n *nopCaller) call(_ context.Context, _ *callRequest) (json.RawMessage, *Usage, error) {
	return json.RawMessage("{}"), &Usage{}, nil
}
