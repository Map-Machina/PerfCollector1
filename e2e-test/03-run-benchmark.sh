#!/bin/bash
# Sysbench Benchmark Script for Azure VM (pcc-test-01)
# Runs various sysbench workloads while capturing performance data

set -e

# Configuration
BENCHMARK_DURATION="${BENCHMARK_DURATION:-300}"  # 5 minutes default
THREADS="${THREADS:-4}"
OUTPUT_DIR="${OUTPUT_DIR:-$HOME/benchmark-results}"
JOURNAL_FILE="${JOURNAL_FILE:-$HOME/.perfprocessord/data/journal}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Sysbench Benchmark Suite ===${NC}"
echo "Configuration:"
echo "  Duration: ${BENCHMARK_DURATION}s per test"
echo "  Threads: $THREADS"
echo "  Output Directory: $OUTPUT_DIR"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Function to check if perfcollectord is running
check_collector() {
    if ! pgrep -f perfcollectord > /dev/null; then
        echo -e "${RED}ERROR: perfcollectord is not running${NC}"
        echo "Please start the collector first:"
        echo "  perfcollectord --sshid=~/.ssh/id_ed25519 --listen=0.0.0.0:2222"
        exit 1
    fi
    echo -e "${GREEN}✓ perfcollectord is running${NC}"
}

# Function to check sysbench
check_sysbench() {
    if ! command -v sysbench &> /dev/null; then
        echo -e "${RED}ERROR: sysbench is not installed${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ sysbench is installed: $(sysbench --version)${NC}"
}

# Function to run benchmark and log results
run_benchmark() {
    local name="$1"
    local cmd="$2"
    local output_file="$OUTPUT_DIR/${name}_$(date +%Y%m%d_%H%M%S).log"

    echo ""
    echo -e "${YELLOW}>>> Running: $name${NC}"
    echo "Command: $cmd"
    echo "Output: $output_file"
    echo ""

    # Run benchmark and capture output
    eval "$cmd" 2>&1 | tee "$output_file"

    echo ""
    echo -e "${GREEN}<<< Completed: $name${NC}"
    echo ""
}

# Warmup function
warmup() {
    echo -e "${YELLOW}Running 30-second warmup...${NC}"
    sysbench cpu --cpu-max-prime=10000 --threads=2 --time=30 run > /dev/null 2>&1
    echo -e "${GREEN}Warmup complete${NC}"
    sleep 5
}

# Pre-flight checks
echo "=== Pre-flight Checks ==="
check_collector
check_sysbench

# Record start time
START_TIME=$(date +%s)
echo ""
echo "Benchmark started at: $(date)"
echo "Expected completion: $(date -d "+$((BENCHMARK_DURATION * 4 + 120)) seconds")"

# Warmup
warmup

# Test 1: CPU-intensive workload
echo ""
echo "============================================"
echo "TEST 1: CPU-Intensive Workload"
echo "============================================"
run_benchmark "cpu_intensive" \
    "sysbench cpu --cpu-max-prime=20000 --threads=$THREADS --time=$BENCHMARK_DURATION run"

# Brief pause between tests
sleep 10

# Test 2: Memory-intensive workload
echo ""
echo "============================================"
echo "TEST 2: Memory-Intensive Workload"
echo "============================================"
run_benchmark "memory_intensive" \
    "sysbench memory --memory-block-size=1M --memory-total-size=100G --memory-oper=write --threads=$THREADS run"

# Brief pause
sleep 10

# Test 3: Mixed CPU and Memory workload
echo ""
echo "============================================"
echo "TEST 3: Mixed CPU + Memory Workload"
echo "============================================"

# Start CPU load in background
echo "Starting background CPU load..."
sysbench cpu --cpu-max-prime=15000 --threads=$((THREADS/2)) --time=$BENCHMARK_DURATION run > "$OUTPUT_DIR/mixed_cpu_$(date +%Y%m%d_%H%M%S).log" 2>&1 &
CPU_PID=$!

# Run memory test in foreground
run_benchmark "mixed_memory" \
    "sysbench memory --memory-block-size=512K --memory-total-size=50G --memory-oper=read --threads=$((THREADS/2)) run"

# Wait for CPU test to complete
wait $CPU_PID 2>/dev/null || true

# Brief pause
sleep 10

# Test 4: Variable load (ramp up/down pattern)
echo ""
echo "============================================"
echo "TEST 4: Variable Load Pattern"
echo "============================================"

RAMP_DURATION=$((BENCHMARK_DURATION / 5))
for load in 25 50 75 100 75 50 25; do
    echo -e "${YELLOW}>>> Load level: ${load}%${NC}"
    ADJUSTED_THREADS=$((THREADS * load / 100))
    [[ $ADJUSTED_THREADS -lt 1 ]] && ADJUSTED_THREADS=1

    sysbench cpu --cpu-max-prime=15000 --threads=$ADJUSTED_THREADS --time=$RAMP_DURATION run \
        >> "$OUTPUT_DIR/variable_load_$(date +%Y%m%d_%H%M%S).log" 2>&1

    sleep 2
done

# Record end time
END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

# Summary
echo ""
echo "============================================"
echo -e "${GREEN}BENCHMARK COMPLETE${NC}"
echo "============================================"
echo ""
echo "Start time: $(date -d @$START_TIME)"
echo "End time: $(date -d @$END_TIME)"
echo "Total duration: ${TOTAL_DURATION}s ($(($TOTAL_DURATION / 60))m $(($TOTAL_DURATION % 60))s)"
echo ""
echo "Results saved to: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR"
echo ""
echo "Journal file: $JOURNAL_FILE"
if [[ -f "$JOURNAL_FILE" ]]; then
    echo "Journal size: $(du -h "$JOURNAL_FILE" | cut -f1)"
else
    echo -e "${YELLOW}Note: Journal file not found at expected location${NC}"
    echo "If using perfcollector_script.sh, check output file location"
fi
echo ""
echo "=== Next Steps ==="
echo "1. Stop the collector if running in sink mode"
echo "2. Copy the journal file to the OCI VM:"
echo "   scp $JOURNAL_FILE perftest-vm-02:~/replay-data/journal"
echo "3. Run CPU training on OCI VM"
echo "4. Execute replay on OCI VM"
EOF
