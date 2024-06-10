package images

import (
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	// EchoServer image
	EchoServer = "ghcr.io/teran/echo-grpc-server:latest"

	// Minio image tag
	Minio = "index.docker.io/minio/minio:RELEASE.2024-05-10T01-41-38Z"

	// Postgres image tag
	Postgres = "index.docker.io/library/postgres:16.3"

	// ScyllaDB image tag
	ScyllaDB = "index.docker.io/scylladb/scylla:5.4.6"
)

func ImageName(image string) string {
	prefix := os.Getenv("IMAGE_PREFIX")
	if prefix != "" {
		image = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(image, "/")
	}

	log.Tracef("image name to pull: %s", image)

	return image
}
