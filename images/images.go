package images

const (
	// EchoServer image
	EchoServer = "ghcr.io/teran/echo-grpc-server:latest"

	// Memcache image tag
	Memcache = "index.docker.io/library/memcached:1.6.29-alpine3.20"

	// MongoDB image tag
	MongoDB = "index.docker.io/library/mongo:7.0.43"

	// ClickHouse image tag
	ClickHouse = "index.docker.io/clickhouse/clickhouse-server:26.8"

	// Kafka image
	Kafka = "index.docker.io/apache/kafka:4.0.0"

	// Minio image tag: PGSTY Silo, a community-maintained fork of MinIO that
	// keeps publishing multi-arch images and security fixes after upstream
	// MinIO stopped distributing community Docker images (see
	// https://github.com/pgsty/silo).
	Minio = "index.docker.io/pgsty/silo:RELEASE.2026-09-03T13-18-01Z"

	// Nginx image tag (Alpine, stable mainline minor pin)
	Nginx = "index.docker.io/library/nginx:1.27-alpine"

	// Postgres image tag
	Postgres = "index.docker.io/library/postgres:16.15"

	// ScyllaDB image tag
	ScyllaDB = "index.docker.io/scylladb/scylla:2026.2.0"

	// RabbitMQ image tag
	RabbitMQ = "index.docker.io/library/rabbitmq:4.0-management"

	// K3s image tag
	K3s = "index.docker.io/rancher/k3s:v1.36.2-k3s1"

	// OpenSearch image tag
	OpenSearch = "index.docker.io/opensearchproject/opensearch:2.19.6"

	// Libvirtd image tag, built and published from
	// github.com/teran/libvirtd-container. Runs libvirtd listening on TCP
	// (16509) without auth for integration testing. Requires /dev/kvm
	// passthrough for KVM acceleration (falls back to TCG software emulation
	// without it).
	Libvirtd = "ghcr.io/teran/libvirtd-container/libvirtd:v0.1.1"

	// Ceph (RGW demo) image tag, published multi-arch (amd64+arm64) from
	// github.com/teran/ceph-container. Images are tagged v<version> for the
	// squid and tentacle release trains.
	Ceph = "ghcr.io/teran/ceph-container/ceph:v20.2.4"

	// Forgejo image tag (rootful, pinned to the 16 release line). Forgejo is
	// a soft fork of Gitea; the image runs the full Forgejo server backed by
	// SQLite for lightweight integration testing.
	Forgejo = "codeberg.org/forgejo/forgejo:16"

	// NetBox image tag: NetBox v4.6 with netbox-docker 5.0.1 support files
	// (release build; netbox-docker tag scheme v<netbox>-<netboxdocker>).
	// NetBox requires external PostgreSQL and Redis — the netbox wrapper spins
	// both up.
	NetBox = "index.docker.io/netboxcommunity/netbox:v4.6-5.0.1"

	// NetBoxPostgres is the PostgreSQL image backing NetBox. Pinned to a stable
	// 16.x release, which is within the range supported by NetBox v4.6 (the
	// netbox-docker compose floats to a newer major, so we pin a known-good
	// stable minor for reproducible tests).
	NetBoxPostgres = "index.docker.io/library/postgres:16.15"

	// NetBoxRedis is the Redis image backing NetBox (single instance serving
	// both the task queue and the cache). Pinned to a stable 7.4-alpine build,
	// which is within the range supported by NetBox v4.6.
	NetBoxRedis = "index.docker.io/library/redis:7.4-alpine"

	// PaperlessNGX image tag: paperless-ngx v3.1.3 (multi-arch amd64+arm64).
	PaperlessNGX = "index.docker.io/paperlessngx/paperless-ngx:3.1.3"

	// PaperlessNGXPostgres is the PostgreSQL image backing Paperless-ngx.
	PaperlessNGXPostgres = "index.docker.io/library/postgres:18"

	// PaperlessNGXValkey is the Valkey (Redis-protocol compatible) message broker
	// backing Paperless-ngx.
	PaperlessNGXValkey = "index.docker.io/valkey/valkey:9-alpine"

	// PaperlessNGXGotenberg converts office documents to PDF (optional, used at
	// consumption time).
	PaperlessNGXGotenberg = "index.docker.io/gotenberg/gotenberg:8.34"

	// PaperlessNGXTika extracts text from office documents (optional, used at
	// consumption time).
	PaperlessNGXTika = "index.docker.io/apache/tika:3.3.1.0"
)
