package browserhttp

import (
	"context"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserinput"
)

// DetailSource returns a bounded detached entity, with no fallback or lazy reads.
type DetailSource interface {
	Detail(context.Context, auth.ReadScope, string, string, string) (browserinput.Detail, error)
}
