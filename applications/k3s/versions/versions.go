// Package versions provides a shared test suite for versioned K3s images.
package versions

import (
	"context"
	"strings"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/teran/go-docker-testsuite/applications/k3s"
)

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

// New creates a new test suite for the given K3s image tag.
func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

// TestK3sVersion verifies that the K3s container starts and the Kubernetes
// server version matches the expected minor version derived from the image tag.
func (s *testSuite) TestK3sVersion() {
	app, err := k3s.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := app.Close(s.ctx)
		s.Require().NoError(err)
	}()

	cs, err := app.Clientset(s.ctx)
	s.Require().NoError(err)

	nodes, err := cs.CoreV1().Nodes().List(s.ctx, metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(nodes.Items, 1)
	s.T().Logf("k3s node: %s", nodes.Items[0].Name)

	sv, err := cs.Discovery().ServerVersion()
	s.Require().NoError(err)
	s.Require().NotEmpty(sv.GitVersion)
	s.T().Logf("k3s version: %s", sv.GitVersion)

	// Verify the server version starts with the expected K8s minor (e.g. "v1.36")
	expectedPrefix := extractK8sMinor(s.image)
	s.Require().True(strings.HasPrefix(sv.GitVersion, expectedPrefix),
		"expected server version %q to have prefix %q (from image %q)",
		sv.GitVersion, expectedPrefix, s.image)
}

// extractK8sMinor extracts the "v1.XX" prefix from a k3s image tag.
// Input example: "index.docker.io/rancher/k3s:v1.36.2-k3s1" → "v1.36"
func extractK8sMinor(image string) string {
	// Find the last colon to get past the registry/repo part
	idx := strings.LastIndex(image, ":")
	if idx == -1 {
		return ""
	}
	tag := image[idx+1:] // e.g. "v1.36.2-k3s1"

	// Split on "." and take first two dot-separated parts
	parts := strings.SplitN(tag, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1] // e.g. "v1.36"
}
