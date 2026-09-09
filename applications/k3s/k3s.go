// Package k3s provides a K3s container for integration testing.
package k3s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	containerName = "k3s"
	apiPort       = 6443

	waitForReadyDelay = 2 * time.Second
)

// K3s represents a running K3s container for integration testing.
type K3s interface {
	// Close stops the container and cleans up the temp kubeconfig file.
	Close(ctx context.Context) error

	// Clientset returns a Kubernetes clientset connected to this K3s instance.
	Clientset(ctx context.Context) (*kubernetes.Clientset, error)

	// KubeconfigPath returns the path to the rewritten kubeconfig file.
	KubeconfigPath() (string, error)

	// ID returns the Docker container ID.
	ID() docker.ContainerID
}

type k3s struct {
	c              docker.Container
	kubeconfigPath string
	kubeconfigData []byte
}

// New creates a new K3s container with the default image.
func New(ctx context.Context) (K3s, error) {
	return NewWithImage(ctx, images.K3s)
}

// NewWithImage creates a new K3s container with a custom image.
func NewWithImage(ctx context.Context, image string) (K3s, error) {
	log.WithFields(log.Fields{
		"image": image,
	}).Debug("creating k3s container")

	c, err := docker.NewContainer(
		containerName,
		image,
		[]string{
			"server",
			"--disable=traefik",
			"--disable=metrics-server",
			"--disable=local-storage",
		},
		docker.NewEnvironment().
			StringVar("K3S_TOKEN", "go-docker-testsuite-secret-token").
			StringVar("K3S_KUBECONFIG_MODE", "644"),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, apiPort),
		docker.WithPrivileged(),
		docker.WithTmpfs(map[string]string{
			"/run": "",
			"/tmp": "",
		}),
		docker.WithBinds("/lib/modules:/lib/modules:ro"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating k3s container")
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running k3s container")
	}

	log.Trace("waiting for k3s readiness: Node controller sync successful")
	if err := c.AwaitOutput(ctx, docker.NewSubstringMatcher("Node controller sync successful")); err != nil {
		return nil, errors.Wrap(err, "error waiting for k3s readiness")
	}

	// Give k3s a moment to fully initialize and write the kubeconfig.
	time.Sleep(waitForReadyDelay)

	res, err := c.Exec(ctx, []string{"cat", "/etc/rancher/k3s/k3s.yaml"})
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving kubeconfig from k3s container")
	}
	if err := res.Error(); err != nil {
		return nil, errors.Wrap(err, "error retrieving kubeconfig from k3s container")
	}
	kubeconfigData := res.Stdout

	k := &k3s{
		c:              c,
		kubeconfigData: kubeconfigData,
	}

	if err := k.rewriteKubeconfig(ctx); err != nil {
		return nil, errors.Wrap(err, "error rewriting kubeconfig")
	}

	started = true
	return k, nil
}

// Close stops the container and cleans up the temp kubeconfig file.
func (k *k3s) Close(ctx context.Context) error {
	if k.kubeconfigPath != "" {
		log.Trace("removing temp kubeconfig file")
		if err := os.Remove(k.kubeconfigPath); err != nil {
			return errors.Wrap(err, "error removing kubeconfig file")
		}
	}

	return k.c.Close(ctx)
}

// Clientset returns a Kubernetes clientset connected to this K3s instance.
func (k *k3s) Clientset(ctx context.Context) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", k.kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "error building kubeconfig")
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "error creating kubernetes clientset")
	}

	return clientset, nil
}

// KubeconfigPath returns the path to the rewritten kubeconfig file.
func (k *k3s) KubeconfigPath() (string, error) {
	return k.kubeconfigPath, nil
}

// ID returns the Docker container ID.
func (k *k3s) ID() docker.ContainerID {
	return k.c.ID()
}

func (k *k3s) rewriteKubeconfig(ctx context.Context) error {
	hp, err := k.c.URL(docker.ProtoTCP, apiPort)
	if err != nil {
		return errors.Wrap(err, "error getting k3s API URL")
	}

	serverAddr := fmt.Sprintf("https://%s", hp.String())

	log.WithFields(log.Fields{
		"server": serverAddr,
	}).Trace("rewriting kubeconfig server address")

	rewritten := strings.ReplaceAll(string(k.kubeconfigData), "https://127.0.0.1:6443", serverAddr)
	rewritten = strings.ReplaceAll(rewritten, "https://localhost:6443", serverAddr)

	tmpDir, err := os.MkdirTemp("", "k3s-kubeconfig-*")
	if err != nil {
		return errors.Wrap(err, "error creating temp directory")
	}

	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig.yaml")
	if err := os.WriteFile(kubeconfigPath, []byte(rewritten), 0600); err != nil {
		return errors.Wrap(err, "error writing kubeconfig file")
	}

	k.kubeconfigPath = kubeconfigPath
	return nil
}
