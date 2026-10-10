package workerhost_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/authorworker"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationauthorworker"
	"github.com/OpenUdon/browsertools/workerhost"
)

// These trusted-host fakes are contract witnesses, not actual containment,
// native runtime qualification, process launch or consumer adoption.
type fakeHost struct {
	events       []string
	channel      *fakeChannel
	mutate       func(*workerhost.Launch)
	launchError  error
	joinError    error
	unjoined     bool
	foreignJoin  bool
	lateJoin     bool
	joinDeadline time.Time
	joinCanceled bool
}

func (h *fakeHost) Launch(ctx context.Context, spec workerhost.LaunchSpec) (workerhost.Launch, error) {
	h.events = append(h.events, "containment", "process")
	h.channel = &fakeChannel{host: h, expectedDeadline: spec.AdmittedDeadline, currentClaim: true}
	launch := workerhost.Launch{LeaseID: "fixture-process", ContainmentLeaseID: "fixture-containment", Spec: spec, AllTraffic: true, Sandbox: true, Channel: h.channel}
	if h.mutate != nil {
		h.mutate(&launch)
	}
	return launch, h.launchError
}

func (h *fakeHost) Join(ctx context.Context, request workerhost.JoinRequest) (workerhost.JoinResult, error) {
	h.events = append(h.events, "join")
	h.joinDeadline, _ = ctx.Deadline()
	h.joinCanceled = ctx.Err() != nil
	if h.lateJoin {
		<-ctx.Done()
	}
	nonce := request.Expected.LaunchNonce
	if h.foreignJoin {
		nonce = "foreign"
	}
	return workerhost.JoinResult{LaunchNonce: nonce, LeaseID: request.Launch.LeaseID, ContainmentLeaseID: request.Launch.ContainmentLeaseID, Joined: !h.unjoined}, h.joinError
}

type fakeChannel struct {
	host             *fakeHost
	expectedDeadline time.Time
	currentClaim     bool
	response         []byte
	closeBlock       chan struct{}
	afterReceive     func()
	sends, closes    int
}

func (c *fakeChannel) Send(ctx context.Context, data []byte) error {
	deadline, ok := ctx.Deadline()
	if !ok || deadline.After(c.expectedDeadline) || ctx.Err() != nil || !c.currentClaim {
		return errors.New("host claim/current-authority refused")
	}
	c.host.events = append(c.host.events, "claimed-send")
	c.sends++
	return nil
}

func (c *fakeChannel) Receive(ctx context.Context) ([]byte, error) {
	if c.afterReceive != nil {
		c.afterReceive()
		return c.response, nil // Deliberately late transport success.
	}
	return c.response, ctx.Err()
}

func (c *fakeChannel) Close() error {
	if c.closeBlock != nil {
		<-c.closeBlock
	}
	c.host.events = append(c.host.events, "close")
	c.closes++
	return nil
}

func launchSpec(protocol string) workerhost.LaunchSpec {
	hash := strings.Repeat("a", 64)
	return workerhost.LaunchSpec{
		Purpose: "capture", Protocol: protocol, BindingSHA256: hash,
		LaunchNonce: "fixture-launch", Origins: []string{"https://example.test"},
		AdmittedDeadline: time.Now().Add(time.Minute), Executable: "/host/verified-worker", Arguments: []string{"private-worker"},
		Closure: workerhost.Closure{
			WorkerModule: "github.com/example/host", WorkerProfile: "kinet.browser.capture.v1", WorkerRevision: strings.Repeat("b", 40), WorkerBinarySHA256: hash,
			WorkerClosureSHA256: hash, DriverRevision: "pinned-playwright-go", DriverClosureSHA256: hash, DriverLockSHA256: hash, DriverBinarySHA256: hash,
			NodeVersion: "24", NodeBinarySHA256: hash, PlaywrightVersion: "1.62.1", PlaywrightClosureSHA256: hash,
			ChromiumVersion: "151.0.7922.34", ChromiumRevision: "1234", ChromiumBinarySHA256: hash, Sandbox: true,
		},
	}
}

func TestManagedHostsPreserveExistingAuthorAndRegistrationProtocols(t *testing.T) {
	protocols := []string{authorsession.Protocol, registrationauthorsession.ProtocolV1, registrationauthorsession.ProtocolV2, registrationauthorsession.ProtocolV3, registrationauthorsession.ProtocolV4}
	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			host := &fakeHost{}
			spec := launchSpec(protocol)
			exchange := func(ctx context.Context, channel workerhost.Channel) error {
				if deadline, _ := ctx.Deadline(); !deadline.Equal(spec.AdmittedDeadline) {
					t.Fatal("extended original admitted deadline")
				}
				// Even a callback's background context cannot extend the launch.
				if err := channel.Send(context.Background(), []byte(`{"protocol":"`+protocol+`","type":"close"}`)); err != nil {
					return err
				}
				host.channel.response = []byte(`{"protocol":"` + protocol + `","type":"hello"}`)
				if _, err := channel.Receive(ctx); err != nil {
					return err
				}
				return channel.Close()
			}
			var err error
			if protocol == authorsession.Protocol {
				err = authorworker.Launch(t.Context(), host, spec, time.Second, exchange)
			} else {
				err = registrationauthorworker.Launch(t.Context(), host, spec, time.Second, exchange)
			}
			if err != nil || strings.Join(host.events, ",") != "containment,process,claimed-send,close,join" || host.channel.closes != 1 || host.joinCanceled {
				t.Fatal("managed lifecycle or wire drift", host.events, err)
			}
		})
	}
}

func TestManagedLaunchRefusesUncontainedOrMismatchedBindingsBeforeInput(t *testing.T) {
	mutations := map[string]func(*workerhost.Launch){
		"all-traffic":       func(l *workerhost.Launch) { l.AllTraffic = false },
		"sandbox":           func(l *workerhost.Launch) { l.Sandbox = false },
		"lease":             func(l *workerhost.Launch) { l.LeaseID = "" },
		"containment-lease": func(l *workerhost.Launch) { l.ContainmentLeaseID = "" },
		"channel":           func(l *workerhost.Launch) { l.Channel = nil },
		"purpose":           func(l *workerhost.Launch) { l.Spec.Purpose = "execute" },
		"protocol":          func(l *workerhost.Launch) { l.Spec.Protocol = "foreign" },
		"binding":           func(l *workerhost.Launch) { l.Spec.BindingSHA256 = strings.Repeat("c", 64) },
		"worker":            func(l *workerhost.Launch) { l.Spec.Closure.WorkerBinarySHA256 = strings.Repeat("c", 64) },
		"driver":            func(l *workerhost.Launch) { l.Spec.Closure.DriverClosureSHA256 = strings.Repeat("c", 64) },
		"nonce":             func(l *workerhost.Launch) { l.Spec.LaunchNonce = "foreign" },
		"origins":           func(l *workerhost.Launch) { l.Spec.Origins[0] = "https://foreign.test" },
		"deadline":          func(l *workerhost.Launch) { l.Spec.AdmittedDeadline = l.Spec.AdmittedDeadline.Add(time.Second) },
		"executable":        func(l *workerhost.Launch) { l.Spec.Executable = "/unapproved/worker" },
		"arguments":         func(l *workerhost.Launch) { l.Spec.Arguments[0] = "foreign" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			host := &fakeHost{mutate: mutate}
			spec := launchSpec(authorsession.Protocol)
			called := false
			err := authorworker.Launch(t.Context(), host, spec, time.Second, func(context.Context, workerhost.Channel) error { called = true; return nil })
			if !errors.Is(err, workerhost.ErrLaunch) || called || host.channel.sends != 0 || host.events[len(host.events)-1] != "join" || spec.Origins[0] != "https://example.test" || spec.Arguments[0] != "private-worker" {
				t.Fatal("invalid binding opened channel, skipped join or mutated caller", called, host.events, err)
			}
		})
	}
}

func TestManagedPartialLaunchAndUnknownJoinPreserveUncertainty(t *testing.T) {
	for _, host := range []*fakeHost{
		{launchError: errors.New("secret/path/backend")},
		{launchError: errors.New("secret/path/backend"), unjoined: true},
		{unjoined: true}, {foreignJoin: true}, {joinError: errors.New("secret/backend")},
	} {
		called := false
		err := authorworker.Launch(t.Context(), host, launchSpec(authorsession.Protocol), time.Second, func(context.Context, workerhost.Channel) error { called = true; return nil })
		if err == nil || strings.Contains(err.Error(), "secret") || host.events[len(host.events)-1] != "join" || host.launchError != nil && called {
			t.Fatal("partial launch or join failed open", called, host.events, err)
		}
		if host.unjoined || host.foreignJoin || host.joinError != nil {
			if !errors.Is(err, workerhost.ErrUnjoined) {
				t.Fatal("lost unjoined quarantine", err)
			}
		}
	}
}

func TestManagedCancellationUsesIndependentBoundedJoinAndFencesSavedChannel(t *testing.T) {
	host := &fakeHost{}
	ctx, cancel := context.WithCancel(context.Background())
	var retained workerhost.Channel
	err := authorworker.Launch(ctx, host, launchSpec(authorsession.Protocol), time.Second, func(ctx context.Context, channel workerhost.Channel) error {
		retained = channel
		cancel()
		return nil // A callback cannot conceal runtime cancellation.
	})
	if !errors.Is(err, context.Canceled) || host.joinCanceled || host.joinDeadline.IsZero() || host.joinDeadline.After(time.Now().Add(time.Second)) {
		t.Fatal("runtime cancellation bypassed separate bounded teardown", host, err)
	}
	if err := retained.Send(context.Background(), []byte("start")); !errors.Is(err, workerhost.ErrTransport) || host.channel.sends != 0 {
		t.Fatal("retained channel replayed old launch", err)
	}
}

func TestManagedTransportBoundsAndMandatoryHostClaim(t *testing.T) {
	host := &fakeHost{}
	err := authorworker.Launch(t.Context(), host, launchSpec(authorsession.Protocol), time.Second, func(ctx context.Context, channel workerhost.Channel) error {
		for _, data := range [][]byte{nil, make([]byte, 1<<20+1)} {
			if err := channel.Send(ctx, data); !errors.Is(err, workerhost.ErrTransport) {
				t.Fatal("sent unbounded native message", err)
			}
		}
		if host.channel.sends != 0 {
			t.Fatal("invalid payload reached host transport")
		}
		host.channel.response = make([]byte, 1<<20+1)
		if data, err := channel.Receive(ctx); !errors.Is(err, workerhost.ErrTransport) || data != nil {
			t.Fatal("returned oversized response", len(data), err)
		}
		host.channel.currentClaim = false
		return channel.Send(ctx, []byte("effect-capable native message"))
	})
	if !errors.Is(err, workerhost.ErrExchange) || host.channel.sends != 0 {
		t.Fatal("missing current claim became transport authority", err)
	}
}

func TestManagedCloseDeadlineCannotBecomeJoinedSuccess(t *testing.T) {
	host := &fakeHost{}
	blocked := make(chan struct{})
	err := authorworker.Launch(t.Context(), host, launchSpec(authorsession.Protocol), 20*time.Millisecond, func(context.Context, workerhost.Channel) error {
		host.channel.closeBlock = blocked
		return nil
	})
	close(blocked) // Settle the synthetic abandoned close after its uncertainty.
	if !errors.Is(err, workerhost.ErrUnjoined) || !host.joinCanceled {
		t.Fatal("close deadline was treated as positive join", err)
	}
}

func TestManagedLateJoinAcknowledgementCannotGrantAcceptance(t *testing.T) {
	host := &fakeHost{lateJoin: true}
	started := time.Now()
	err := authorworker.Launch(t.Context(), host, launchSpec(authorsession.Protocol), 20*time.Millisecond, func(context.Context, workerhost.Channel) error { return nil })
	if !errors.Is(err, workerhost.ErrUnjoined) || time.Since(started) < 20*time.Millisecond {
		t.Fatal("late joined acknowledgement bypassed original teardown budget", err)
	}
}

func TestManagedLateReceivedResponseIsFencedAfterCancellation(t *testing.T) {
	host := &fakeHost{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := authorworker.Launch(ctx, host, launchSpec(authorsession.Protocol), time.Second, func(ctx context.Context, channel workerhost.Channel) error {
		host.channel.response = []byte("late-private-native-response")
		host.channel.afterReceive = cancel
		data, err := channel.Receive(ctx)
		if data != nil || !errors.Is(err, workerhost.ErrTransport) {
			t.Fatal("late response escaped current runtime fence", string(data), err)
		}
		return err
	})
	if !errors.Is(err, workerhost.ErrExchange) || host.joinCanceled {
		t.Fatal("late response or runtime cancellation skipped independent join", err)
	}
}

func TestManagedMissingCapabilityAndUnknownProtocolHaveNoLegacyFallback(t *testing.T) {
	for _, protocol := range []string{"", "v1", "browsertools.author-session.v1", "foreign.v9"} {
		host := &fakeHost{}
		called := false
		exchange := func(context.Context, workerhost.Channel) error { called = true; return nil }
		if err := authorworker.Launch(t.Context(), host, launchSpec(protocol), time.Second, exchange); !errors.Is(err, workerhost.ErrLaunch) {
			t.Fatal(err)
		}
		if err := registrationauthorworker.Launch(t.Context(), host, launchSpec(protocol), time.Second, exchange); !errors.Is(err, workerhost.ErrLaunch) || called || len(host.events) != 0 {
			t.Fatal("protocol invoked legacy fallback", called, host.events, err)
		}
	}
	for _, mutate := range []func(*workerhost.LaunchSpec){
		func(s *workerhost.LaunchSpec) { s.Closure.WorkerBinarySHA256 = "" },
		func(s *workerhost.LaunchSpec) { s.Closure.Sandbox = false },
		func(s *workerhost.LaunchSpec) { s.Purpose = "execute" },
		func(s *workerhost.LaunchSpec) { s.AdmittedDeadline = time.Now().Add(-time.Second) },
		func(s *workerhost.LaunchSpec) { s.Origins = []string{"https://example.test:443"} },
	} {
		spec, host := launchSpec(authorsession.Protocol), &fakeHost{}
		mutate(&spec)
		if err := authorworker.Launch(t.Context(), host, spec, time.Second, func(context.Context, workerhost.Channel) error { return nil }); !errors.Is(err, workerhost.ErrLaunch) || len(host.events) != 0 {
			t.Fatal("unsupported launch used host capability", host.events, err)
		}
	}
	if err := authorworker.Launch(t.Context(), nil, launchSpec(authorsession.Protocol), time.Second, func(context.Context, workerhost.Channel) error { return nil }); !errors.Is(err, workerhost.ErrLaunch) {
		t.Fatal("missing containment capability accepted", err)
	}
}
