#!/bin/bash
# Master E2E Test Orchestration Script
# Run this from your local machine to coordinate the test across both VMs

set -e

# Configuration - UPDATE THESE VALUES
AZURE_VM="pcc-test-01"           # Azure VM hostname or IP
AZURE_USER="${AZURE_USER:-azureuser}"
OCI_VM="perftest-vm-02"           # OCI VM hostname or IP
OCI_USER="${OCI_USER:-opc}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"

# Test parameters
BENCHMARK_DURATION="${BENCHMARK_DURATION:-300}"  # 5 minutes
TEST_MODE="${TEST_MODE:-simple}"  # simple or encrypted

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

print_header() {
    echo ""
    echo -e "${CYAN}${BOLD}======================================${NC}"
    echo -e "${CYAN}${BOLD}  $1${NC}"
    echo -e "${CYAN}${BOLD}======================================${NC}"
    echo ""
}

print_step() {
    echo -e "${YELLOW}>>> $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# SSH wrapper functions
ssh_azure() {
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${AZURE_USER}@${AZURE_VM}" "$@"
}

ssh_oci() {
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${OCI_USER}@${OCI_VM}" "$@"
}

scp_azure() {
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$@"
}

scp_oci() {
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$@"
}

# Check connectivity
check_connectivity() {
    print_header "Phase 0: Connectivity Check"

    print_step "Testing connection to Azure VM ($AZURE_VM)..."
    if ssh_azure "echo 'Connected to Azure VM'" 2>/dev/null; then
        print_success "Azure VM accessible"
    else
        print_error "Cannot connect to Azure VM"
        echo "Please check:"
        echo "  - VM hostname/IP: $AZURE_VM"
        echo "  - SSH user: $AZURE_USER"
        echo "  - SSH key: $SSH_KEY"
        exit 1
    fi

    print_step "Testing connection to OCI VM ($OCI_VM)..."
    if ssh_oci "echo 'Connected to OCI VM'" 2>/dev/null; then
        print_success "OCI VM accessible"
    else
        print_error "Cannot connect to OCI VM"
        echo "Please check:"
        echo "  - VM hostname/IP: $OCI_VM"
        echo "  - SSH user: $OCI_USER"
        echo "  - SSH key: $SSH_KEY"
        exit 1
    fi
}

# Deploy scripts to VMs
deploy_scripts() {
    print_header "Phase 1: Deploy Test Scripts"

    print_step "Deploying scripts to Azure VM..."
    scp_azure "$SCRIPT_DIR/01-setup-azure.sh" "${AZURE_USER}@${AZURE_VM}:~/"
    scp_azure "$SCRIPT_DIR/03-run-benchmark.sh" "${AZURE_USER}@${AZURE_VM}:~/"
    scp_azure "$SCRIPT_DIR/04-capture-simple.sh" "${AZURE_USER}@${AZURE_VM}:~/"
    ssh_azure "chmod +x ~/*.sh"
    print_success "Scripts deployed to Azure VM"

    print_step "Deploying scripts to OCI VM..."
    scp_oci "$SCRIPT_DIR/02-setup-oci.sh" "${OCI_USER}@${OCI_VM}:~/"
    scp_oci "$SCRIPT_DIR/05-validate-replay.sh" "${OCI_USER}@${OCI_VM}:~/"
    ssh_oci "chmod +x ~/*.sh"
    print_success "Scripts deployed to OCI VM"
}

# Setup VMs
setup_vms() {
    print_header "Phase 2: Setup VMs"

    print_step "Setting up Azure VM..."
    ssh_azure "bash ~/01-setup-azure.sh" || true
    print_success "Azure VM setup complete"

    print_step "Setting up OCI VM..."
    ssh_oci "bash ~/02-setup-oci.sh" || true
    print_success "OCI VM setup complete"
}

# Run benchmark and capture
run_capture() {
    print_header "Phase 3: Run Benchmark and Capture"

    print_step "Starting benchmark with capture on Azure VM..."
    echo "Duration: ${BENCHMARK_DURATION}s"
    echo ""

    if [[ "$TEST_MODE" == "simple" ]]; then
        # Simple unencrypted capture
        ssh_azure "CAPTURE_DURATION=$BENCHMARK_DURATION bash ~/04-capture-simple.sh" &
        CAPTURE_PID=$!

        # Wait a few seconds for capture to start
        sleep 5

        # Run benchmark
        print_step "Running sysbench benchmark..."
        ssh_azure "BENCHMARK_DURATION=$BENCHMARK_DURATION bash ~/03-run-benchmark.sh"

        # Wait for capture to finish
        wait $CAPTURE_PID 2>/dev/null || true
    else
        # Encrypted capture with perfcollectord/perfprocessord
        echo "Encrypted mode not yet implemented in this script"
        echo "Please use manual setup for encrypted capture"
        exit 1
    fi

    print_success "Benchmark and capture complete"
}

# Transfer data
transfer_data() {
    print_header "Phase 4: Transfer Data to OCI VM"

    print_step "Copying captured data from Azure to OCI..."

    # Create temp directory
    TEMP_DIR=$(mktemp -d)

    # Download from Azure
    scp_azure "${AZURE_USER}@${AZURE_VM}:~/benchmark-capture.json" "$TEMP_DIR/journal"

    # Upload to OCI
    ssh_oci "mkdir -p ~/replay-data"
    scp_oci "$TEMP_DIR/journal" "${OCI_USER}@${OCI_VM}:~/replay-data/journal"

    # Cleanup
    rm -rf "$TEMP_DIR"

    print_success "Data transferred to OCI VM"
}

# Run CPU training
run_training() {
    print_header "Phase 5: CPU Training on OCI VM"

    print_step "Running CPU training (this takes ~2 minutes)..."
    ssh_oci "cd ~/replay-data && perfcpumeasure --siteid=1 --host=0 -v > training.json 2>&1" | tee /tmp/training_output.txt

    print_success "CPU training complete"

    echo ""
    echo "Training results:"
    ssh_oci "cat ~/replay-data/training.json"
}

# Run replay and validate
run_replay() {
    print_header "Phase 6: Replay and Validation"

    print_step "Starting replay with validation..."
    ssh_oci "bash ~/05-validate-replay.sh"

    print_success "Replay and validation complete"
}

# Collect results
collect_results() {
    print_header "Phase 7: Collect Results"

    RESULTS_DIR="$SCRIPT_DIR/results_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$RESULTS_DIR"

    print_step "Downloading results..."

    # From Azure
    scp_azure "${AZURE_USER}@${AZURE_VM}:~/benchmark-results/*" "$RESULTS_DIR/" 2>/dev/null || true
    scp_azure "${AZURE_USER}@${AZURE_VM}:~/benchmark-capture.json" "$RESULTS_DIR/original_capture.json" 2>/dev/null || true

    # From OCI
    scp_oci "${OCI_USER}@${OCI_VM}:~/replay-data/training.json" "$RESULTS_DIR/" 2>/dev/null || true
    scp_oci "${OCI_USER}@${OCI_VM}:~/validation-results/*" "$RESULTS_DIR/" 2>/dev/null || true

    print_success "Results saved to: $RESULTS_DIR"

    echo ""
    echo "Files collected:"
    ls -la "$RESULTS_DIR"
}

# Print summary
print_summary() {
    print_header "Test Summary"

    echo "Test Mode: $TEST_MODE"
    echo "Benchmark Duration: ${BENCHMARK_DURATION}s"
    echo ""
    echo "Azure VM: $AZURE_VM"
    echo "OCI VM: $OCI_VM"
    echo ""

    if [[ -d "$RESULTS_DIR" ]]; then
        echo "Results directory: $RESULTS_DIR"

        if [[ -f "$RESULTS_DIR/validation_report_"*.txt ]]; then
            echo ""
            echo "Validation Report:"
            echo "=================="
            cat "$RESULTS_DIR/validation_report_"*.txt
        fi
    fi
}

# Main menu
show_menu() {
    echo ""
    echo -e "${BOLD}PerfCollector E2E Test${NC}"
    echo "======================"
    echo ""
    echo "Test Configuration:"
    echo "  Azure VM: $AZURE_VM (user: $AZURE_USER)"
    echo "  OCI VM:   $OCI_VM (user: $OCI_USER)"
    echo "  SSH Key:  $SSH_KEY"
    echo "  Duration: ${BENCHMARK_DURATION}s"
    echo ""
    echo "Options:"
    echo "  1) Run full E2E test"
    echo "  2) Check connectivity only"
    echo "  3) Deploy scripts only"
    echo "  4) Run capture only (Azure)"
    echo "  5) Run training only (OCI)"
    echo "  6) Run replay only (OCI)"
    echo "  7) Collect results"
    echo "  q) Quit"
    echo ""
    read -p "Select option: " choice

    case $choice in
        1)
            check_connectivity
            deploy_scripts
            setup_vms
            run_capture
            transfer_data
            run_training
            run_replay
            collect_results
            print_summary
            ;;
        2) check_connectivity ;;
        3) deploy_scripts ;;
        4) run_capture ;;
        5) run_training ;;
        6) run_replay ;;
        7) collect_results ;;
        q|Q) exit 0 ;;
        *) echo "Invalid option" ;;
    esac
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --azure-vm) AZURE_VM="$2"; shift 2 ;;
        --azure-user) AZURE_USER="$2"; shift 2 ;;
        --oci-vm) OCI_VM="$2"; shift 2 ;;
        --oci-user) OCI_USER="$2"; shift 2 ;;
        --ssh-key) SSH_KEY="$2"; shift 2 ;;
        --duration) BENCHMARK_DURATION="$2"; shift 2 ;;
        --auto) AUTO_RUN=1; shift ;;
        --help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --azure-vm <host>    Azure VM hostname/IP (default: pcc-test-01)"
            echo "  --azure-user <user>  Azure SSH user (default: azureuser)"
            echo "  --oci-vm <host>      OCI VM hostname/IP (default: perftest-vm-02)"
            echo "  --oci-user <user>    OCI SSH user (default: opc)"
            echo "  --ssh-key <path>     SSH key path (default: ~/.ssh/id_rsa)"
            echo "  --duration <secs>    Benchmark duration in seconds (default: 300)"
            echo "  --auto               Run full test automatically"
            echo "  --help               Show this help"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Run automatically or show menu
if [[ "$AUTO_RUN" == "1" ]]; then
    check_connectivity
    deploy_scripts
    setup_vms
    run_capture
    transfer_data
    run_training
    run_replay
    collect_results
    print_summary
else
    show_menu
fi
EOF
