# go-docker-testsuite

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
