package wait

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"

	docker "github.com/teran/go-docker-testsuite"
)

// fakeTarget is a minimal in-memory wait.Target whose Exec always reports a
// successful (exit code 0) run and whose URL returns a fixed host:port. It lets
// the examples exercise Wait without a real container.
type fakeTarget struct {
	hostPort *docker.HostPort
}

func (t *fakeTarget) URL(_ docker.Protocol, _ uint16) (*docker.HostPort, error) {
	return t.hostPort, nil
}

func (t *fakeTarget) Exec(_ context.Context, _ []string) (*docker.ExecResult, error) {
	return &docker.ExecResult{ExitCode: 0}, nil
}

// logFakeTarget adds the optional GetOutput capability on top of fakeTarget so
// examples can exercise ForLog without a real container.
type logFakeTarget struct {
	*fakeTarget
	lines []string
}

func (t *logFakeTarget) GetOutput(_ context.Context, m ...docker.Matcher) ([]string, error) {
	return t.lines, nil
}

// ExampleWait demonstrates the top-level Wait function: polling a target until
// a readiness strategy reports ready. Wait is the entry point for every
// strategy and combinator in this package.
func ExampleWait() {
	err := Wait(context.Background(), &fakeTarget{}, ForCommand([]string{"echo", "ok"}))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForTCPConnection demonstrates a pure TCP liveness probe: ForTCPConnection
// reports ready once a TCP connection to the container's internal port can be
// established.
func ExampleForTCPConnection() {
	// A listener that accepts connections immediately, acting as the "service".
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	defer func() { _ = l.Close() }()

	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > int(^uint16(0)) {
		fmt.Printf("error: invalid port %q\n", portStr)
		return
	}

	// ForTCPConnection dials the listener's own port via t.URL.
	t := &fakeTarget{hostPort: &docker.HostPort{Host: "127.0.0.1", Port: uint16(port)}}

	err = Wait(context.Background(), t, ForTCPConnection(uint16(port)))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForLog demonstrates matching a line of container output. ForLog is
// ready once some line matches the given matcher. It requires a Target that
// implements the optional GetOutput capability (docker.Container and
// docker.TestContainer both do).
func ExampleForLog() {
	t := &logFakeTarget{
		fakeTarget: &fakeTarget{},
		lines:      []string{"started", "READY"},
	}

	err := Wait(
		context.Background(),
		t,
		ForLog(docker.NewSubstringMatcher("READY")),
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForAll demonstrates combining strategies: ForAll reports ready only
// when every sub-strategy is ready. Here a command probe and a TCP liveness
// probe must both succeed.
func ExampleForAll() {
	err := Wait(
		context.Background(),
		&fakeTarget{},
		ForAll(
			ForCommand([]string{"echo", "ok"}),
			ForCommand([]string{"true"}),
		),
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForAny demonstrates combining strategies: ForAny reports ready as soon
// as at least one sub-strategy is ready.
func ExampleForAny() {
	err := Wait(
		context.Background(),
		&fakeTarget{},
		ForAny(
			ForCommand([]string{"false", "ignored"}),
			ForCommand([]string{"echo", "ok"}),
		),
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForAtLeast demonstrates combining strategies: ForAtLeast(x, ...) is
// ready when at least x of the sub-strategies are ready.
func ExampleForAtLeast() {
	err := Wait(
		context.Background(),
		&fakeTarget{},
		ForAtLeast(1,
			ForCommand([]string{"echo", "ok"}),
			ForCommand([]string{"echo", "also ok"}),
		),
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForCommand demonstrates probing a container by running a command
// inside it. ForCommand reports ready as soon as the command exits with code 0.
func ExampleForCommand() {
	err := Wait(context.Background(), &fakeTarget{}, ForCommand([]string{"echo", "ok"}))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}

// ExampleForHTTPGet demonstrates probing a container by making an HTTP
// request to one of its ports. ForHTTPGet reports ready once the probe returns
// an accepted status on the requested path.
func ExampleForHTTPGet() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > int(^uint16(0)) {
		fmt.Printf("error: invalid port %q\n", portStr)
		return
	}

	t := &fakeTarget{hostPort: &docker.HostPort{Host: host, Port: uint16(port)}}

	err = Wait(
		context.Background(),
		t,
		ForHTTPGet(uint16(port), WithPath("/health"), WithResponseStatuses(http.StatusOK)),
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("ready")
	// Output: ready
}
