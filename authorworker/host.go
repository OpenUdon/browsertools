package authorworker

import (
	"context"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/workerhost"
)

// Launch lets a host such as Kinet manage one separately contained author
// worker. The host selects its stable image and supplies exact current policy,
// claims and lifecycle capabilities. Existing Run is the explicit retained
// worker-process entry; Launch never invokes it as an uncontained fallback.
func Launch(ctx context.Context, host workerhost.Host, spec workerhost.LaunchSpec, teardownTimeout time.Duration, exchange func(context.Context, workerhost.Channel) error) error {
	if spec.Protocol != authorsession.Protocol {
		return workerhost.ErrLaunch
	}
	return workerhost.RunManaged(ctx, host, spec, teardownTimeout, exchange)
}
