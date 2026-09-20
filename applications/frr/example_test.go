package frr_test

import (
	"context"
	"fmt"
	"time"

	"github.com/teran/go-docker-testsuite/applications/frr"
)

// Example demonstrates starting an FRR container with an injected config and
// checking whether a BGP prefix is present in its table.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// A minimal config declaring a BGP instance and pointing a peer at
	// itself (loopback) so the daemon has something to talk to.
	config := []byte(`hostname frr
frr defaults traditional
router bgp 65000
 bgp router-id 10.0.0.1
 neighbor 10.0.0.1 remote-as 65000
 neighbor 10.0.0.1 update-source 10.0.0.1
!
`)

	app, err := frr.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	// The prefix is not announced yet, so this is false — but the call itself
	// succeeds, which proves vtysh can reach the FRR daemons.
	has, err := app.HasBGPRoute(ctx, "10.0.0.0/24")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Println("prefix present:", has)
}
