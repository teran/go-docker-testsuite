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
