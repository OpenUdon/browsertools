// Package workerhost defines the managed capture-process boundary for hosts
// such as Kinet. It implements no browser launch, network policy, claims,
// credential resolution, session custody or runtime/report semantics.
package workerhost

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/profile"
)

var (
	ErrLaunch    = errors.New("managed browser capture launch refused")
	ErrExchange  = errors.New("managed browser capture exchange failed; dispatch outcome is not attested")
	ErrUnjoined  = errors.New("managed browser capture join unproved; retain and quarantine custody")
	ErrTransport = errors.New("managed browser capture channel refused")
)

// Closure uses the frozen M51 identity field names. The host verifies the
// actual installed artifacts and exact approved profile before Launch. Merely
// filling this metadata does not establish installation, trust or authority.
type Closure struct {
	WorkerModule, WorkerProfile, WorkerRevision, WorkerBinarySHA256           string
	WorkerClosureSHA256, DriverRevision, DriverClosureSHA256                  string
	DriverLockSHA256, DriverBinarySHA256                                      string
	NodeVersion, NodeBinarySHA256, PlaywrightVersion, PlaywrightClosureSHA256 string
	ChromiumVersion, ChromiumRevision, ChromiumBinarySHA256                   string
	Sandbox                                                                   bool
}

// LaunchSpec is supplied by trusted host code, never by model/profile/request
// bytes. BindingSHA256 is the host's independently issued capture identity;
// the host owns current approval/owner/agent/claim checks. Purpose is capture.
// Private handles carry only host-selected installation/display/result-root
// configuration, never runtime credential or retained-session values.
type LaunchSpec struct {
	Purpose              string
	Protocol             string
	BindingSHA256        string
	Closure              Closure
	LaunchNonce          string
	Origins              []string
	AdmittedDeadline     time.Time
	Executable           string   `json:"-"`
	Arguments            []string `json:"-"`
	PrivateDisplayHandle any      `json:"-"`
	PrivateResultHandle  any      `json:"-"`
}

// Channel carries existing private native NDJSON. Every method must honor
// cancellation; Close must interrupt pending reads/writes. The host adapter
// checks current exact authority and obtains any required durable dispatch
// claim immediately before effect-capable sends. Send success or failure is
// transport evidence only, never proof of business effects or non-dispatch.
type Channel interface {
	Send(context.Context, []byte) error
	Receive(context.Context) ([]byte, error)
	Close() error
}

// Launch retains partial custody even on error. No channel is exposed to the
// exchange until the host attests the exact binding and all-traffic containment.
type Launch struct {
	LeaseID, ContainmentLeaseID string
	Spec                        LaunchSpec
	AllTraffic, Sandbox         bool
	Channel                     Channel `json:"-"`
}

type JoinRequest struct {
	Expected LaunchSpec
	Launch   Launch
}

// Joined means all preparatory processes, worker, Node/Chromium descendants,
// channel readers and host callbacks are positively joined for this nonce and
// custody, including partial launch failures. It says nothing about effects.
// A host returns false on missing or uncertain teardown and retains quarantine.
type JoinResult struct {
	LaunchNonce, LeaseID, ContainmentLeaseID string
	Joined                                   bool
}

// Host is an explicit trusted capture capability. Launch must establish
// default-closed all-traffic containment before starting the selected process
// tree, or refuse. This includes every independent route and unsupported
// channel; navigation guards and visited URLs are insufficient. Join closes
// transport and joins stable process identities before releasing containment.
// M56 owns the real implementation and qualification, not this package.
type Host interface {
	Launch(context.Context, LaunchSpec) (Launch, error)
	Join(context.Context, JoinRequest) (JoinResult, error)
}

// RunManaged exposes one capture channel only after a matching contained
// Launch. It never falls back to a legacy worker entry. The exchange uses the
// original admitted deadline; teardown has a separate finite context, permits
// no sends, and still runs after cancellation or partial launch failure.
// TeardownTimeout is positive and at most30seconds. A callback must join its
// own work before returning; Host.Join also settles host-owned readers.
func RunManaged(ctx context.Context, host Host, spec LaunchSpec, teardownTimeout time.Duration, exchange func(context.Context, Channel) error) error {
	if ctx == nil || host == nil || exchange == nil || teardownTimeout <= 0 || teardownTimeout > 30*time.Second || !validSpec(spec) {
		return ErrLaunch
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrLaunch, err)
	}
	spec.Origins = slices.Clone(spec.Origins)
	spec.Arguments = slices.Clone(spec.Arguments)
	runtimeCtx, cancel := context.WithDeadline(ctx, spec.AdmittedDeadline)
	defer cancel()
	// Give the host independent slice ownership; mutation cannot redefine the
	// expected binding later used by the guard or Join.
	launchSpec := spec
	launchSpec.Origins = slices.Clone(spec.Origins)
	launchSpec.Arguments = slices.Clone(spec.Arguments)
	launch, launchErr := host.Launch(runtimeCtx, launchSpec)
	var result error
	var guarded *guardedChannel
	if launch.Channel != nil {
		guarded = &guardedChannel{channel: launch.Channel, runtime: runtimeCtx, closeDone: make(chan struct{})}
	}
	if launchErr != nil || !validLaunch(spec, launch) || runtimeCtx.Err() != nil {
		result = errors.Join(ErrLaunch, runtimeCtx.Err())
	} else {
		if err := exchange(runtimeCtx, guarded); err != nil || runtimeCtx.Err() != nil {
			result = errors.Join(ErrExchange, runtimeCtx.Err())
		}
	}
	// Stop new sends before channel close and independent teardown. In-flight
	// sends are conservatively uncertain and must be settled by the host Join.
	if guarded != nil {
		guarded.closed.Store(true)
	}
	cancel()
	teardownCtx, finish := context.WithTimeout(context.Background(), teardownTimeout)
	defer finish()
	closeUnproved := false
	if guarded != nil {
		if err := guarded.close(teardownCtx); err != nil {
			result = errors.Join(result, ErrTransport)
			closeUnproved = teardownCtx.Err() != nil
		}
	}
	joined, err := host.Join(teardownCtx, JoinRequest{Expected: spec, Launch: launch})
	if closeUnproved || teardownCtx.Err() != nil || err != nil || !joined.Joined || joined.LaunchNonce != spec.LaunchNonce || joined.LeaseID != launch.LeaseID || joined.ContainmentLeaseID != launch.ContainmentLeaseID {
		return errors.Join(result, ErrUnjoined)
	}
	return result
}

func validLaunch(expected LaunchSpec, launch Launch) bool {
	actual := launch.Spec
	return safeText(launch.LeaseID, 256) && safeText(launch.ContainmentLeaseID, 256) && launch.Channel != nil && launch.AllTraffic && launch.Sandbox &&
		actual.Purpose == expected.Purpose && actual.Protocol == expected.Protocol && actual.BindingSHA256 == expected.BindingSHA256 && actual.Closure == expected.Closure &&
		actual.LaunchNonce == expected.LaunchNonce && actual.AdmittedDeadline.Equal(expected.AdmittedDeadline) && slices.Equal(actual.Origins, expected.Origins) &&
		actual.Executable == expected.Executable && slices.Equal(actual.Arguments, expected.Arguments)
}

func validSpec(spec LaunchSpec) bool {
	if spec.Purpose != "capture" || !safeText(spec.Protocol, 128) || !digest(spec.BindingSHA256) || !safeText(spec.LaunchNonce, 256) || !time.Now().Before(spec.AdmittedDeadline) || !filepath.IsAbs(spec.Executable) || !safeText(spec.Executable, 4096) || len(spec.Arguments) > 64 || len(spec.Origins) == 0 || len(spec.Origins) > 64 || !spec.Closure.Sandbox {
		return false
	}
	c := spec.Closure
	for _, value := range []string{c.WorkerModule, c.WorkerProfile, c.WorkerRevision, c.DriverRevision, c.NodeVersion, c.PlaywrightVersion, c.ChromiumVersion, c.ChromiumRevision} {
		if !safeText(value, 256) {
			return false
		}
	}
	for _, value := range []string{c.WorkerBinarySHA256, c.WorkerClosureSHA256, c.DriverClosureSHA256, c.DriverLockSHA256, c.DriverBinarySHA256, c.NodeBinarySHA256, c.PlaywrightClosureSHA256, c.ChromiumBinarySHA256} {
		if !digest(value) {
			return false
		}
	}
	for _, arg := range spec.Arguments {
		if !safeText(arg, 4096) {
			return false
		}
	}
	for i, origin := range spec.Origins {
		canonical, err := profile.ParseOrigin(origin)
		if err != nil || canonical != origin || i > 0 && spec.Origins[i-1] >= origin {
			return false
		}
	}
	return true
}

func safeText(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func digest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type guardedChannel struct {
	channel   Channel
	runtime   context.Context
	closed    atomic.Bool
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

func (c *guardedChannel) callContext(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil || c.closed.Load() || c.runtime.Err() != nil {
		return nil, nil, errors.Join(ErrTransport, c.runtime.Err())
	}
	deadline, _ := c.runtime.Deadline()
	bounded, cancel := context.WithDeadline(ctx, deadline)
	stop := context.AfterFunc(c.runtime, cancel)
	return bounded, func() { stop(); cancel() }, nil
}

func (c *guardedChannel) Send(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > 1<<20 {
		return ErrTransport
	}
	bounded, cancel, err := c.callContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if err := c.channel.Send(bounded, data); err != nil {
		return err
	}
	if c.closed.Load() || c.runtime.Err() != nil || bounded.Err() != nil {
		return ErrTransport
	}
	return nil
}

func (c *guardedChannel) Receive(ctx context.Context) ([]byte, error) {
	bounded, cancel, err := c.callContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	data, err := c.channel.Receive(bounded)
	if err != nil {
		return nil, err
	}
	if c.closed.Load() || c.runtime.Err() != nil || bounded.Err() != nil {
		return nil, ErrTransport
	}
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, ErrTransport
	}
	return data, nil
}

func (c *guardedChannel) Close() error {
	return c.close(c.runtime)
}

func (c *guardedChannel) close(ctx context.Context) error {
	c.closed.Store(true)
	c.closeOnce.Do(func() {
		go func() {
			c.closeErr = c.channel.Close()
			close(c.closeDone)
		}()
	})
	select {
	case <-c.closeDone:
		if c.closeErr != nil {
			return ErrTransport
		}
		return nil
	case <-ctx.Done():
		return errors.Join(ErrTransport, ctx.Err())
	}
}
