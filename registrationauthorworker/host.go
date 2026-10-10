package registrationauthorworker

import (
	"context"
	"time"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/workerhost"
)

// Launch lets a host such as Kinet manage one separately contained no-submit
// registration author worker. The full existing v1–v4 wire discriminator is
// mandatory; there is no inferred protocol, browser or legacy Run fallback.
// Host capabilities own exact current authority, claims and teardown.
func Launch(ctx context.Context, host workerhost.Host, spec workerhost.LaunchSpec, teardownTimeout time.Duration, exchange func(context.Context, workerhost.Channel) error) error {
	switch spec.Protocol {
	case registrationauthorsession.ProtocolV1, registrationauthorsession.ProtocolV2, registrationauthorsession.ProtocolV3, registrationauthorsession.ProtocolV4:
		return workerhost.RunManaged(ctx, host, spec, teardownTimeout, exchange)
	default:
		return workerhost.ErrLaunch
	}
}
