# Source this for local development and integration tests.
#
# Finch runs a Docker-API-compatible daemon; Testcontainers (Go, Node, Python,
# Rust) picks it up from DOCKER_HOST. Start the VM first with `finch vm start`.
export DOCKER_HOST="unix:///Applications/Finch/lima/data/finch/sock/finch.sock"
export TESTCONTAINERS_RYUK_DISABLED=true
export CONTAINER_RUNTIME=finch
export GOTOOLCHAIN=auto
