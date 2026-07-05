package vault_test

import (
	"context"
	"fmt"
	"time"

	vaultSDK "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"

	"github.com/teran/go-docker-testsuite/applications/vault"
)

// This example demonstrates starting a Vault container, retrieving the root
// token, creating a KV v2 secrets engine, and reading/writing secrets.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := vault.New(ctx, "index.docker.io/hashicorp/vault:1.21.0")
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer app.Close(ctx)

	rootToken, err := app.GetRootToken(ctx)
	if err != nil {
		fmt.Printf("error getting root token: %v\n", err)
		return
	}
	fmt.Println("vault started, root token:", rootToken)

	if err := app.CreateEngine(ctx, "mysecrets", "kv-v2"); err != nil {
		fmt.Printf("error creating engine: %v\n", err)
		return
	}
	fmt.Println("engine created")

	client, err := app.GetRootClient(ctx)
	if err != nil {
		fmt.Printf("error getting client: %v\n", err)
		return
	}

	if _, err := client.Secrets.KvV2Write(ctx, "config", schema.KvV2WriteRequest{
		Data: map[string]any{
			"key": "value",
		},
	}, vaultSDK.WithMountPath("mysecrets")); err != nil {
		fmt.Printf("error writing secret: %v\n", err)
		return
	}
	fmt.Println("secret written")

	sec, err := client.Secrets.KvV2Read(ctx, "config", vaultSDK.WithMountPath("mysecrets"))
	if err != nil {
		fmt.Printf("error reading secret: %v\n", err)
		return
	}
	fmt.Printf("secret value: %s\n", sec.Data.Data["key"])

	if err := app.RemoveEngine(ctx, "mysecrets"); err != nil {
		fmt.Printf("error removing engine: %v\n", err)
		return
	}
	fmt.Println("engine removed")
}
