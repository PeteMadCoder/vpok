#!/usr/bin/env bash
# ==============================================================================
# VPOK - VIRTUAL PACKAGE ORCHESTRATION KIT: AUTOMATED TEST RUNNER
# ==============================================================================
#
# TESTING INFRASTRUCTURE GUIDELINES & RULES:
# ------------------------------------------------------------------------------
# 1. NON-DISRUPTIVE ISOLATION:
#    - Tests MUST NOT interfere with or disrupt any running development instances.
#    - Provisions an isolated test environment under /tmp/vpok_test_XXXXXX.
#    - Uses a dedicated Unix socket (vpokd.sock), isolated CAS store directory,
#      dedicated signing keypair directory, and independent runtime workspaces.
#    - Never touches or pollutes the user's ~/.vpok/ state.
#
# 2. REPRODUCIBILITY & IDEMPOTENCY:
#    - Tests MUST produce identical, verifiable outcomes on repeated runs.
#    - Residual test processes and sockets are purged prior to execution.
#    - Automated EXIT/SIGINT/SIGTERM trap tears down background processes and
#      wipes isolated artifacts upon completion.
#
# 3. FAULT TOLERANCE & CI COMPLIANCE:
#    - Does NOT use 'set -e'; executes the entire suite even if early steps fail.
#    - Failed steps are recorded in the FAILED_STEPS array.
#    - Outputs a consolidated summary report at the end with standard exit codes
#      (0 on total success, 1 on any failure) for CI pipelines.
#
# 4. HUMAN-READABLE OUTPUT:
#    - Formatted with standardized color-coded prefixes: [INFO], [SUCCESS],
#      [WARNING], and [ERROR].
#    - Displays test output in real-time for immediate observability.
# ==============================================================================

set -u

# ANSI Color Formatting
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FAILED_STEPS=()
OVERALL_SUCCESS=true
TEST_PIDS=()

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "\n${YELLOW}====================================================${NC}"
    echo -e "${YELLOW}${BOLD}  $1${NC}"
    echo -e "${YELLOW}====================================================${NC}\n"
}

print_header "VPOK Automated System & Regression Test Suite"

# ------------------------------------------------------------------------------
# Test Environment Configuration (Completely Isolated from Host / Dev)
# ------------------------------------------------------------------------------
TEST_TMP_DIR=$(mktemp -d /tmp/vpok_test_XXXXXX)
TEST_BIN_DIR="${TEST_TMP_DIR}/bin"
TEST_STORE_DIR="${TEST_TMP_DIR}/store"
TEST_SOCKET_PATH="${TEST_TMP_DIR}/vpokd.sock"
TEST_KEYS_DIR="${TEST_TMP_DIR}/keys"
TEST_WORK_DIR="${TEST_TMP_DIR}/work"
TEST_RUNTIME_DIR="${TEST_TMP_DIR}/runtime"

mkdir -p "${TEST_BIN_DIR}" "${TEST_STORE_DIR}" "${TEST_KEYS_DIR}" "${TEST_WORK_DIR}" "${TEST_RUNTIME_DIR}"

log_info "Isolated test workspace created at: ${TEST_TMP_DIR}"
log_info "Dedicated test CAS store:           ${TEST_STORE_DIR}"
log_info "Dedicated test Unix domain socket:  ${TEST_SOCKET_PATH}"
log_info "Dedicated test keys directory:      ${TEST_KEYS_DIR}"

# ------------------------------------------------------------------------------
# Cleanup Trap for Isolated Test Infrastructure and Artifacts
# ------------------------------------------------------------------------------
cleanup() {
    local exit_code=$?
    echo ""
    log_info "Cleaning up isolated test infrastructure (workspace: ${TEST_TMP_DIR})..."

    # 1. Terminate tracked background test processes
    for pid in "${TEST_PIDS[@]}"; do
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done

    # 2. Remove isolated Unix domain socket if lingering
    if [ -S "$TEST_SOCKET_PATH" ]; then
        rm -f "$TEST_SOCKET_PATH"
    fi

    # 3. Wipe isolated test directory
    if [ -d "$TEST_TMP_DIR" ]; then
        rm -rf "$TEST_TMP_DIR"
    fi

    log_success "Isolated test infrastructure cleanly destroyed. Active host state untouched."
    exit "$exit_code"
}
trap cleanup EXIT INT TERM

# ------------------------------------------------------------------------------
# Step 1: Tooling & Environment Verification
# ------------------------------------------------------------------------------
print_header "Step 1: Tooling & Environment Verification"
TOOLING_VALID=true

for tool in go curl tar; do
    if command -v "$tool" &>/dev/null; then
        log_success "Required tool '$tool' is available ($(command -v "$tool"))."
    else
        log_error "Required tool '$tool' is MISSING from PATH!"
        TOOLING_VALID=false
    fi
done

# Hypervisor presence check (non-fatal warning if QEMU/KVM not installed)
if command -v qemu-system-x86_64 &>/dev/null; then
    log_success "Hypervisor binary 'qemu-system-x86_64' found."
else
    log_warning "'qemu-system-x86_64' not found in PATH; hardware VM execution will fall back or mock."
fi

if [ -e "/dev/kvm" ] && [ -r "/dev/kvm" ] && [ -w "/dev/kvm" ]; then
    log_success "Hardware acceleration (/dev/kvm) is accessible."
else
    log_warning "/dev/kvm is not accessible; QEMU tests will run under software emulation."
fi

if [ "$TOOLING_VALID" = false ]; then
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Tooling & Environment Check")
fi

# ------------------------------------------------------------------------------
# Step 2: Compile Binaries into Isolated Test Directory
# ------------------------------------------------------------------------------
print_header "Step 2: Isolated Binary Compilation"
log_info "Compiling project binaries into ${TEST_BIN_DIR}..."

BINARIES_TO_BUILD=(
    "vpok:./cmd/vpok"
    "vpokd:./cmd/vpokd"
    "vpok-agent:./cmd/vpok-agent"
)

BUILD_FAIL=false
for item in "${BINARIES_TO_BUILD[@]}"; do
    name="${item%%:*}"
    pkg="${item##*:}"
    log_info "Building ${name} (${pkg})..."
    if go build -o "${TEST_BIN_DIR}/${name}" "${pkg}"; then
        log_success "Compiled binary '${name}' successfully."
    else
        log_error "Compilation failed for package '${pkg}'!"
        BUILD_FAIL=true
    fi
done

if [ "$BUILD_FAIL" = true ]; then
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Isolated Binary Compilation")
fi

# ------------------------------------------------------------------------------
# Step 3: Go Unit & Subsystem Tests
# ------------------------------------------------------------------------------
print_header "Step 3: Go Unit & Subsystem Tests"
log_info "Executing all package unit tests with real-time verbose output..."

if go test -v ./...; then
    log_success "All Go package unit tests passed."
else
    log_error "One or more Go unit tests failed!"
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Go Package Unit Tests (go test ./...)")
fi

# ------------------------------------------------------------------------------
# Step 4: Keypair Generation & Security Permissions Test
# ------------------------------------------------------------------------------
print_header "Step 4: Cryptographic Keypair Generation Test"
log_info "Testing 'vpok keygen' with isolated output directory..."

if "${TEST_BIN_DIR}/vpok" keygen --out "${TEST_KEYS_DIR}"; then
    PRIV_KEY="${TEST_KEYS_DIR}/publisher.key"
    PUB_KEY="${TEST_KEYS_DIR}/publisher.pub"

    if [ -f "$PRIV_KEY" ] && [ -f "$PUB_KEY" ]; then
        log_success "Keypair successfully generated."

        # Verify strict private key file permissions (0600)
        KEY_PERMS=$(stat -c "%a" "$PRIV_KEY" 2>/dev/null || stat -f "%Lp" "$PRIV_KEY" 2>/dev/null || echo "unknown")
        if [ "$KEY_PERMS" = "600" ]; then
            log_success "Private key permissions verified: mode 0600."
        else
            log_error "Private key insecure permissions: expected 600, got ${KEY_PERMS}!"
            OVERALL_SUCCESS=false
            FAILED_STEPS+=("Keypair File Permissions (0600)")
        fi
    else
        log_error "Key files missing after keygen execution!"
        OVERALL_SUCCESS=false
        FAILED_STEPS+=("Keypair File Generation")
    fi
else
    log_error "'vpok keygen' command failed!"
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("CLI Keygen Execution")
fi

# ------------------------------------------------------------------------------
# Step 5: Build Specification Validation & Package Build Pipeline
# ------------------------------------------------------------------------------
print_header "Step 5: Build Specification & CAS Packaging Test"

# Create a sample build specification and source payload
SAMPLE_CTX="${TEST_WORK_DIR}/pkg_context"
mkdir -p "${SAMPLE_CTX}/bin"
echo '#!/bin/sh' > "${SAMPLE_CTX}/bin/hello"
echo 'echo "Hello from VPOK test package"' >> "${SAMPLE_CTX}/bin/hello"
chmod +x "${SAMPLE_CTX}/bin/hello"

SAMPLE_BUILD_TOML="${TEST_WORK_DIR}/vpok.build.toml"
cat << 'EOF' > "$SAMPLE_BUILD_TOML"
apiVersion = "vpok.io/v1"
kind = "Build"

[metadata]
name = "runner-test-pkg"
version = "0.1.0"
publisher = "vpok.test"
description = "Test runner automated package"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[[build.steps]]
action = "copy"
from = "./bin/hello"
to = "/usr/local/bin/hello"
mode = "0755"

[entrypoint]
command = ["/usr/local/bin/hello"]

[requires.resources]
memory = "64MiB"
cpus = 1
EOF

log_info "Validating build specification..."
if "${TEST_BIN_DIR}/vpok" validate --build "$SAMPLE_BUILD_TOML"; then
    log_success "Build specification validated successfully."
else
    log_error "Failed to validate build specification!"
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Build Spec Validation")
fi

log_info "Building signed package into isolated CAS store..."
BUILD_OUTPUT=$("${TEST_BIN_DIR}/vpok" build -f "$SAMPLE_BUILD_TOML" -C "$SAMPLE_CTX" --store "$TEST_STORE_DIR" --key "${TEST_KEYS_DIR}/publisher.key" 2>&1)
echo "$BUILD_OUTPUT"

MANIFEST_DIGEST=$(echo "$BUILD_OUTPUT" | grep "Manifest Digest:" | awk '{print $3}')

if [ -n "$MANIFEST_DIGEST" ]; then
    log_success "Package built successfully. Manifest digest: ${MANIFEST_DIGEST}"

    # Verify inspection of the built manifest in CAS
    log_info "Inspecting manifest in CAS store via 'vpok inspect'..."
    if "${TEST_BIN_DIR}/vpok" inspect --store "$TEST_STORE_DIR" "$MANIFEST_DIGEST" | grep -q '"runner-test-pkg"'; then
        log_success "'vpok inspect' retrieved and parsed the manifest correctly."
    else
        log_error "'vpok inspect' failed to parse manifest ${MANIFEST_DIGEST}!"
        OVERALL_SUCCESS=false
        FAILED_STEPS+=("CAS Manifest Inspection")
    fi
else
    log_error "Package build failed or did not return a manifest digest!"
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Package Build & CAS Storage")
fi

# ------------------------------------------------------------------------------
# Step 6: Isolated Host Daemon (vpokd) Live Socket & REST API Verification
# ------------------------------------------------------------------------------
print_header "Step 6: Host Daemon (vpokd) Live Socket & API Test"

log_info "Starting isolated vpokd daemon on socket ${TEST_SOCKET_PATH}..."
"${TEST_BIN_DIR}/vpokd" -socket "${TEST_SOCKET_PATH}" -store "${TEST_STORE_DIR}" > "${TEST_TMP_DIR}/vpokd.log" 2>&1 &
VPOKD_PID=$!
TEST_PIDS+=($VPOKD_PID)

# Wait for Unix socket to become ready
DAEMON_READY=false
for _ in {1..20}; do
    if [ -S "$TEST_SOCKET_PATH" ]; then
        if curl -s --unix-socket "$TEST_SOCKET_PATH" http://localhost/v1/ping | grep -q '"status":"ok"'; then
            DAEMON_READY=true
            break
        fi
    fi
    sleep 0.2
done

if [ "$DAEMON_READY" = true ]; then
    log_success "vpokd daemon is running and responsive on ${TEST_SOCKET_PATH}."

    # Test GET /v1/ping
    PING_RESP=$(curl -s --unix-socket "$TEST_SOCKET_PATH" http://localhost/v1/ping)
    if echo "$PING_RESP" | grep -q '"version"'; then
        log_success "GET /v1/ping returned valid health payload."
    else
        log_error "GET /v1/ping returned unexpected payload: ${PING_RESP}"
        OVERALL_SUCCESS=false
        FAILED_STEPS+=("Daemon REST API (/v1/ping)")
    fi

    # Test GET /v1/manifests
    MANIFESTS_RESP=$(curl -s --unix-socket "$TEST_SOCKET_PATH" http://localhost/v1/manifests)
    if echo "$MANIFESTS_RESP" | grep -q "runner-test-pkg"; then
        log_success "GET /v1/manifests indexed the newly built package manifest."
    else
        log_error "GET /v1/manifests failed to index built package! Response: ${MANIFESTS_RESP}"
        OVERALL_SUCCESS=false
        FAILED_STEPS+=("Daemon REST API (/v1/manifests)")
    fi

    # Test GET /v1/manifests/{digest}
    if [ -n "$MANIFEST_DIGEST" ]; then
        DIGEST_RESP=$(curl -s --unix-socket "$TEST_SOCKET_PATH" "http://localhost/v1/manifests/${MANIFEST_DIGEST}")
        if echo "$DIGEST_RESP" | grep -q '"apiVersion":"vpok.io/v1"'; then
            log_success "GET /v1/manifests/{digest} returned canonical manifest."
        else
            log_error "GET /v1/manifests/{digest} failed to return manifest! Response: ${DIGEST_RESP}"
            OVERALL_SUCCESS=false
            FAILED_STEPS+=("Daemon REST API (/v1/manifests/{digest})")
        fi
    fi
else
    log_error "vpokd failed to start or socket timed out! Daemon log output:"
    cat "${TEST_TMP_DIR}/vpokd.log" 2>/dev/null || true
    OVERALL_SUCCESS=false
    FAILED_STEPS+=("Daemon Socket Initialization")
fi

# ------------------------------------------------------------------------------
# Final Summary Banner & CI Exit Code
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}====================================================${NC}"
if [ "$OVERALL_SUCCESS" = true ]; then
    log_success "${BOLD}ALL VPOK TESTS AND VALIDATIONS PASSED SUCCESSFULLY!${NC}"
    echo -e "${YELLOW}====================================================${NC}\n"
    exit 0
else
    log_error "${BOLD}TEST SUITE ENCOUNTERED FAILURES! The following ${#FAILED_STEPS[@]} step(s) failed:${NC}"
    for step in "${FAILED_STEPS[@]}"; do
        echo -e "  ${RED}- $step${NC}"
    done
    echo -e "${YELLOW}====================================================${NC}\n"
    exit 1
fi
