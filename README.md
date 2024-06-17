# go-docker-testsuite

![Test & Build status](https://github.com/teran/go-docker-testsuite/actions/workflows/verify.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/teran/go-docker-testsuite)](https://goreportcard.com/report/github.com/teran/go-docker-testsuite)
[![Go Reference](https://pkg.go.dev/badge/github.com/teran/go-docker-testsuite.svg)](https://pkg.go.dev/github.com/teran/go-docker-testsuite)

Library to run any third-party dependency in Docker on any platform Docker supports
The main purpose is to allow runnings integration tests against almost any
database or other dependency running right within docker and available via
network.

## Applications

The test suite provides some of applications, i.e. wrappers for particular
docker image, here's the list:

* MinIO
* PostgreSQL
* Redis
* ScyllaDB

Each application could provide its own interface to interact so please refer
to applications package for some examples.
