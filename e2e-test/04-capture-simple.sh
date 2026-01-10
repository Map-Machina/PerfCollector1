#!/bin/bash
# Simple capture script using perfcollector_script.sh
# This captures performance data WITHOUT encryption for easier testing
# Run this on the Azure VM (pcc-test-01)

set -e

# Configuration
CAPTURE_DURATION="${CAPTURE_DURATION:-600}"  # 10 minutes default
OUTPUT_FILE="${OUTPUT_FILE:-$HOME/benchmark-capture.json}"
COLLECTION_INTERVAL="${COLLECTION_INTERVAL:-5}"  # 5 seconds

echo "=== Simple Performance Capture ==="
echo "Configuration:"
echo "  Duration: ${CAPTURE_DURATION}s"
echo "  Output: $OUTPUT_FILE"
echo "  Collection Interval: ${COLLECTION_INTERVAL}s"
echo ""

# Check for jq (required by perfcollector_script.sh)
if ! command -v jq &> /dev/null; then
    echo "Installing jq..."
    if command -v apt-get &> /dev/null; then
        sudo apt-get update && sudo apt-get install -y jq
    elif command -v yum &> /dev/null; then
        sudo yum install -y jq
    elif command -v dnf &> /dev/null; then
        sudo dnf install -y jq
    else
        echo "ERROR: Please install jq manually"
        exit 1
    fi
fi

# Create inline collection script (simplified version)
cat > /tmp/collect_perf.sh << 'COLLECT_SCRIPT'
#!/bin/bash
# Inline performance collection script

INTERVAL=${1:-5}
SITEID=${2:-1}
HOSTID=${3:-0}
RUN=${4:-0}

while true; do
    TIMESTAMP=$(date -Iseconds)
    START=$(date +%s%N)

    # Collect /proc/stat
    STAT_CONTENT=$(cat /proc/stat)
    echo "{\"Site\":$SITEID,\"Host\":$HOSTID,\"Run\":$RUN,\"Measurement\":{\"Timestamp\":\"$TIMESTAMP\",\"System\":\"/proc/stat\",\"Measurement\":$(echo "$STAT_CONTENT" | jq -Rs .)}}"

    # Collect /proc/meminfo
    MEMINFO_CONTENT=$(cat /proc/meminfo)
    echo "{\"Site\":$SITEID,\"Host\":$HOSTID,\"Run\":$RUN,\"Measurement\":{\"Timestamp\":\"$TIMESTAMP\",\"System\":\"/proc/meminfo\",\"Measurement\":$(echo "$MEMINFO_CONTENT" | jq -Rs .)}}"

    # Collect /proc/diskstats
    DISKSTATS_CONTENT=$(cat /proc/diskstats)
    echo "{\"Site\":$SITEID,\"Host\":$HOSTID,\"Run\":$RUN,\"Measurement\":{\"Timestamp\":\"$TIMESTAMP\",\"System\":\"/proc/diskstats\",\"Measurement\":$(echo "$DISKSTATS_CONTENT" | jq -Rs .)}}"

    # Collect /proc/net/dev
    NETDEV_CONTENT=$(cat /proc/net/dev)
    echo "{\"Site\":$SITEID,\"Host\":$HOSTID,\"Run\":$RUN,\"Measurement\":{\"Timestamp\":\"$TIMESTAMP\",\"System\":\"/proc/net/dev\",\"Measurement\":$(echo "$NETDEV_CONTENT" | jq -Rs .)}}"

    sleep $INTERVAL
done
COLLECT_SCRIPT
chmod +x /tmp/collect_perf.sh

echo "Starting capture at $(date)"
echo "Will run for ${CAPTURE_DURATION} seconds..."
echo "Output file: $OUTPUT_FILE"
echo ""
echo "Press Ctrl+C to stop early"
echo ""

# Start collection in background, with timeout
timeout $CAPTURE_DURATION /tmp/collect_perf.sh $COLLECTION_INTERVAL 1 0 0 > "$OUTPUT_FILE" 2>/dev/null &
COLLECT_PID=$!

# Wait for collection to complete or be interrupted
wait $COLLECT_PID 2>/dev/null || true

# Count collected samples
if [[ -f "$OUTPUT_FILE" ]]; then
    SAMPLE_COUNT=$(wc -l < "$OUTPUT_FILE")
    FILE_SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)

    echo ""
    echo "=== Capture Complete ==="
    echo "Output file: $OUTPUT_FILE"
    echo "File size: $FILE_SIZE"
    echo "Total samples: $SAMPLE_COUNT"
    echo ""
    echo "Preview (first 5 lines):"
    head -5 "$OUTPUT_FILE" | jq -c '.Measurement.System'
    echo ""
    echo "=== Next Steps ==="
    echo "1. Copy to OCI VM:"
    echo "   scp $OUTPUT_FILE perftest-vm-02:~/replay-data/journal"
    echo ""
    echo "2. On OCI VM, run CPU training:"
    echo "   perfcpumeasure --siteid=1 --host=0 -v > training.json"
    echo ""
    echo "3. Run replay:"
    echo "   perfreplay --siteid=1 --host=0 --run=0 \\"
    echo "     --input=~/replay-data/journal \\"
    echo "     --training=training.json \\"
    echo "     --diskmapper=diskmapper.json"
else
    echo "ERROR: Output file not created"
    exit 1
fi
EOF
