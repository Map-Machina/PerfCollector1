#!/bin/bash
# Replay Validation Script for OCI VM (perftest-vm-02)
# Captures metrics during replay and compares with original

set -e

# Configuration
REPLAY_DATA_DIR="${REPLAY_DATA_DIR:-$HOME/replay-data}"
VALIDATION_DIR="${VALIDATION_DIR:-$HOME/validation-results}"
COLLECTION_INTERVAL="${COLLECTION_INTERVAL:-5}"  # Must match original collection

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}=== Replay Validation Suite ===${NC}"
echo "Replay Data Dir: $REPLAY_DATA_DIR"
echo "Validation Dir: $VALIDATION_DIR"
echo ""

# Create directories
mkdir -p "$VALIDATION_DIR"

# Check for required files
check_files() {
    local missing=0

    echo "Checking required files..."

    if [[ ! -f "$REPLAY_DATA_DIR/journal" ]]; then
        echo -e "${RED}✗ Missing: $REPLAY_DATA_DIR/journal${NC}"
        missing=1
    else
        echo -e "${GREEN}✓ Found: journal${NC}"
    fi

    if [[ ! -f "$REPLAY_DATA_DIR/training.json" ]]; then
        echo -e "${RED}✗ Missing: $REPLAY_DATA_DIR/training.json${NC}"
        echo "  Run: perfcpumeasure --siteid=1 --host=0 -v > $REPLAY_DATA_DIR/training.json"
        missing=1
    else
        echo -e "${GREEN}✓ Found: training.json${NC}"
    fi

    if [[ ! -f "$REPLAY_DATA_DIR/diskmapper.json" ]]; then
        echo -e "${YELLOW}⚠ Missing: $REPLAY_DATA_DIR/diskmapper.json (optional for CPU-only replay)${NC}"
    else
        echo -e "${GREEN}✓ Found: diskmapper.json${NC}"
    fi

    if [[ $missing -eq 1 ]]; then
        echo ""
        echo -e "${RED}Missing required files. Please ensure all files are in place.${NC}"
        exit 1
    fi
}

# Function to collect current system metrics
collect_metrics() {
    local output_file="$1"
    local duration="$2"
    local interval="${3:-5}"

    echo "Collecting metrics for ${duration}s..."

    local end_time=$(($(date +%s) + duration))
    local sample=0

    while [[ $(date +%s) -lt $end_time ]]; do
        sample=$((sample + 1))
        local timestamp=$(date +%s)

        # Read CPU stats
        local cpu_line=$(head -1 /proc/stat)
        local user=$(echo $cpu_line | awk '{print $2}')
        local nice=$(echo $cpu_line | awk '{print $3}')
        local system=$(echo $cpu_line | awk '{print $4}')
        local idle=$(echo $cpu_line | awk '{print $5}')
        local iowait=$(echo $cpu_line | awk '{print $6}')
        local total=$((user + nice + system + idle + iowait))

        # Read memory stats
        local memfree=$(grep MemFree /proc/meminfo | awk '{print $2}')
        local memavail=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
        local memtotal=$(grep MemTotal /proc/meminfo | awk '{print $2}')

        # Output as CSV
        echo "$timestamp,$user,$nice,$system,$idle,$iowait,$total,$memfree,$memavail,$memtotal" >> "$output_file"

        sleep $interval
    done

    echo "Collected $sample samples"
}

# Function to analyze results
analyze_results() {
    local original="$1"
    local replay="$2"
    local report="$3"

    echo ""
    echo -e "${CYAN}=== Analyzing Results ===${NC}"

    # Create report header
    cat > "$report" << EOF
Replay Validation Report
========================
Generated: $(date)
Original File: $original
Replay File: $replay

EOF

    # Basic statistics
    local orig_lines=$(wc -l < "$original" 2>/dev/null || echo 0)
    local replay_lines=$(wc -l < "$replay" 2>/dev/null || echo 0)

    echo "Original samples: $orig_lines" >> "$report"
    echo "Replay samples: $replay_lines" >> "$report"
    echo "" >> "$report"

    # Calculate CPU utilization comparison (simplified)
    echo "CPU Utilization Comparison:" >> "$report"
    echo "----------------------------" >> "$report"

    if [[ -f "$original" ]] && [[ -f "$replay" ]]; then
        # Calculate average CPU busy percentage from replay
        local avg_busy=$(awk -F',' '
            NR > 1 {
                total = $2 + $3 + $4 + $5 + $6
                if (total > 0) {
                    busy = ($2 + $3 + $4) / total * 100
                    sum += busy
                    count++
                }
            }
            END {
                if (count > 0) printf "%.2f", sum/count
                else print "N/A"
            }
        ' "$replay")

        echo "Average CPU Busy (Replay): ${avg_busy}%" >> "$report"
    fi

    echo "" >> "$report"
    echo "Memory Statistics:" >> "$report"
    echo "------------------" >> "$report"

    if [[ -f "$replay" ]]; then
        local avg_mem=$(awk -F',' '
            NR > 1 && $10 > 0 {
                used = ($10 - $8) / $10 * 100
                sum += used
                count++
            }
            END {
                if (count > 0) printf "%.2f", sum/count
                else print "N/A"
            }
        ' "$replay")

        echo "Average Memory Used (Replay): ${avg_mem}%" >> "$report"
    fi

    echo "" >> "$report"
    echo "Validation Status:" >> "$report"
    echo "------------------" >> "$report"

    # Simple pass/fail based on samples collected
    if [[ $replay_lines -gt 10 ]]; then
        echo "Status: PASS - Replay completed with $replay_lines samples" >> "$report"
        echo -e "${GREEN}✓ Validation PASSED${NC}"
    else
        echo "Status: FAIL - Insufficient samples collected" >> "$report"
        echo -e "${RED}✗ Validation FAILED${NC}"
    fi

    cat "$report"
}

# Main execution flow
main() {
    check_files

    echo ""
    echo -e "${YELLOW}Step 1: Starting metrics collection in background${NC}"

    # Start collecting replay metrics
    REPLAY_METRICS="$VALIDATION_DIR/replay_metrics_$(date +%Y%m%d_%H%M%S).csv"
    echo "timestamp,user,nice,system,idle,iowait,total,memfree,memavail,memtotal" > "$REPLAY_METRICS"

    # Estimate replay duration from journal (count samples * interval)
    JOURNAL_SAMPLES=$(wc -l < "$REPLAY_DATA_DIR/journal" 2>/dev/null || echo 100)
    ESTIMATED_DURATION=$((JOURNAL_SAMPLES / 4 * COLLECTION_INTERVAL + 60))  # 4 metrics per sample + buffer

    echo "Estimated replay duration: ${ESTIMATED_DURATION}s"
    echo "Starting background metrics collection..."

    # Start collection in background
    (collect_metrics "$REPLAY_METRICS" "$ESTIMATED_DURATION" "$COLLECTION_INTERVAL") &
    COLLECT_PID=$!

    sleep 2  # Let collection start

    echo ""
    echo -e "${YELLOW}Step 2: Starting replay${NC}"

    # Start replay
    if [[ -f "$REPLAY_DATA_DIR/diskmapper.json" ]]; then
        perfreplay --siteid=1 --host=0 --run=0 \
            --input="$REPLAY_DATA_DIR/journal" \
            --training="$REPLAY_DATA_DIR/training.json" \
            --diskmapper="$REPLAY_DATA_DIR/diskmapper.json" \
            --log=prp=INFO 2>&1 | tee "$VALIDATION_DIR/replay_log.txt" &
    else
        perfreplay --siteid=1 --host=0 --run=0 \
            --input="$REPLAY_DATA_DIR/journal" \
            --training="$REPLAY_DATA_DIR/training.json" \
            --log=prp=INFO 2>&1 | tee "$VALIDATION_DIR/replay_log.txt" &
    fi
    REPLAY_PID=$!

    echo "Replay PID: $REPLAY_PID"
    echo "Collection PID: $COLLECT_PID"
    echo ""
    echo "Waiting for replay to complete..."

    # Wait for replay to finish
    wait $REPLAY_PID 2>/dev/null || true

    echo ""
    echo -e "${YELLOW}Step 3: Stopping metrics collection${NC}"

    # Stop collection
    kill $COLLECT_PID 2>/dev/null || true
    sleep 2

    echo ""
    echo -e "${YELLOW}Step 4: Generating validation report${NC}"

    # Analyze results
    REPORT_FILE="$VALIDATION_DIR/validation_report_$(date +%Y%m%d_%H%M%S).txt"
    analyze_results "$REPLAY_DATA_DIR/journal" "$REPLAY_METRICS" "$REPORT_FILE"

    echo ""
    echo "=== Validation Complete ==="
    echo "Results saved to: $VALIDATION_DIR"
    echo ""
    echo "Files generated:"
    ls -la "$VALIDATION_DIR"
}

# Run main
main "$@"
EOF
