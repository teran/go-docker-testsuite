package libvirtd_test

import (
	"context"
	"fmt"
	"time"

	"github.com/teran/go-docker-testsuite/applications/libvirtd"
)

// This example demonstrates starting a libvirtd container and retrieving the
// libvirt version via the connected go-libvirt client.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := libvirtd.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running, with /dev/kvm?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	ver, err := app.Client().ConnectGetLibVersion()
	if err != nil {
		fmt.Printf("error getting libvirt version: %v\n", err)
		return
	}
	fmt.Printf("libvirt version: %d\n", ver)
}
