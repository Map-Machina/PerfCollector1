#!/bin/bash
# Setup script for OCI VM (perftest-vm-02) - Target/Replay system
# This script prepares the OCI VM for CPU training and workload replay

set -e

# Configuration
PERFCOLLECTOR_HOME="${PERFCOLLECTOR_HOME:-$HOME/perfcollector}"
REPLAY_DATA_DIR="${REPLAY_DATA_DIR:-$HOME/replay-data}"
DISK_REPLAY_DIR="${DISK_REPLAY_DIR:-$HOME/disk-replay}"

echo "=== PerfCollector OCI VM Setup (perftest-vm-02) ==="
echo "Configuration:"
echo "  PERFCOLLECTOR_HOME: $PERFCOLLECTOR_HOME"
echo "  REPLAY_DATA_DIR: $REPLAY_DATA_DIR"
echo "  DISK_REPLAY_DIR: $DISK_REPLAY_DIR"
echo ""

# Check if running on Linux
if [[ "$(uname)" != "Linux" ]]; then
    echo "ERROR: This script must be run on Linux"
    exit 1
fi

# Create directories
echo "[1/5] Creating directories..."
mkdir -p "$PERFCOLLECTOR_HOME"/{bin,config,data}
mkdir -p "$REPLAY_DATA_DIR"
mkdir -p "$DISK_REPLAY_DIR"/{sda,sdb,sdc}

# Check if binaries exist
echo "[2/5] Checking PerfCollector binaries..."
BINARIES_NEEDED="perfcpumeasure perfreplay perfjournal"
MISSING_BINARIES=""

for bin in $BINARIES_NEEDED; do
    if [[ ! -f "$PERFCOLLECTOR_HOME/bin/$bin" ]] && ! command -v $bin &> /dev/null; then
        MISSING_BINARIES="$MISSING_BINARIES $bin"
    fi
done

if [[ -n "$MISSING_BINARIES" ]]; then
    echo "WARNING: Missing binaries:$MISSING_BINARIES"
    echo "Please build and copy binaries to $PERFCOLLECTOR_HOME/bin/"
    echo ""
    echo "Build commands:"
    echo "  cd /path/to/perfcollector"
    echo "  GOOS=linux GOARCH=amd64 go build -o perfcpumeasure ./cmd/perfcpumeasure"
    echo "  GOOS=linux GOARCH=amd64 go build -o perfreplay ./cmd/perfreplay"
    echo "  GOOS=linux GOARCH=amd64 go build -o perfjournal ./cmd/perfjournal"
fi

# Create default disk mapper configuration
echo "[3/5] Creating disk mapper configuration..."
cat > "$REPLAY_DATA_DIR/diskmapper.json" << EOF
{"siteid":1,"host":0,"devicename":"sda","mountpoint":"$DISK_REPLAY_DIR/sda","readsize":"100 mib"}
{"siteid":1,"host":0,"devicename":"sda1","mountpoint":"$DISK_REPLAY_DIR/sda","readsize":"100 mib"}
{"siteid":1,"host":0,"devicename":"sdb","mountpoint":"$DISK_REPLAY_DIR/sdb","readsize":"100 mib"}
{"siteid":1,"host":0,"devicename":"sdb1","mountpoint":"$DISK_REPLAY_DIR/sdb","readsize":"100 mib"}
{"siteid":1,"host":0,"devicename":"sdc","mountpoint":"$DISK_REPLAY_DIR/sdc","readsize":"100 mib"}
{"siteid":1,"host":0,"devicename":"sdc1","mountpoint":"$DISK_REPLAY_DIR/sdc","readsize":"100 mib"}
EOF
echo "Disk mapper written to: $REPLAY_DATA_DIR/diskmapper.json"

# Create helper scripts
echo "[4/5] Creating helper scripts..."

# CPU training script
cat > "$PERFCOLLECTOR_HOME/bin/run-cpu-training.sh" << EOF
#!/bin/bash
# Run CPU training to generate calibration data
# This must be run on the TARGET system before replay

SITEID=\${1:-1}
HOSTID=\${2:-0}
OUTPUT=\${3:-$REPLAY_DATA_DIR/training.json}

echo "Running CPU training..."
echo "Site ID: \$SITEID"
echo "Host ID: \$HOSTID"
echo "Output: \$OUTPUT"
echo ""

perfcpumeasure --siteid=\$SITEID --host=\$HOSTID -v > "\$OUTPUT"

echo ""
echo "Training complete. Output saved to: \$OUTPUT"
cat "\$OUTPUT"
EOF
chmod +x "$PERFCOLLECTOR_HOME/bin/run-cpu-training.sh"

# Replay script
cat > "$PERFCOLLECTOR_HOME/bin/run-replay.sh" << EOF
#!/bin/bash
# Run workload replay from captured journal

JOURNAL=\${1:-$REPLAY_DATA_DIR/journal}
TRAINING=\${2:-$REPLAY_DATA_DIR/training.json}
DISKMAPPER=\${3:-$REPLAY_DATA_DIR/diskmapper.json}
SITEID=\${4:-1}
HOSTID=\${5:-0}
RUNID=\${6:-0}

if [[ ! -f "\$JOURNAL" ]]; then
    echo "ERROR: Journal file not found: \$JOURNAL"
    echo "Usage: \$0 <journal> [training.json] [diskmapper.json] [siteid] [hostid] [runid]"
    exit 1
fi

if [[ ! -f "\$TRAINING" ]]; then
    echo "ERROR: Training file not found: \$TRAINING"
    echo "Run CPU training first: run-cpu-training.sh"
    exit 1
fi

echo "Starting replay..."
echo "  Journal: \$JOURNAL"
echo "  Training: \$TRAINING"
echo "  Disk Mapper: \$DISKMAPPER"
echo "  Site ID: \$SITEID"
echo "  Host ID: \$HOSTID"
echo "  Run ID: \$RUNID"
echo ""

# For unencrypted journals (from perfcollector_script.sh)
perfreplay --siteid=\$SITEID --host=\$HOSTID --run=\$RUNID \\
    --input="\$JOURNAL" \\
    --training="\$TRAINING" \\
    --diskmapper="\$DISKMAPPER" \\
    --log=prp=INFO
EOF
chmod +x "$PERFCOLLECTOR_HOME/bin/run-replay.sh"

# Replay with encryption script
cat > "$PERFCOLLECTOR_HOME/bin/run-replay-encrypted.sh" << EOF
#!/bin/bash
# Run workload replay from encrypted journal

JOURNAL=\${1:-$REPLAY_DATA_DIR/journal}
TRAINING=\${2:-$REPLAY_DATA_DIR/training.json}
DISKMAPPER=\${3:-$REPLAY_DATA_DIR/diskmapper.json}
SITEID=\${4:-1}
SITENAME=\${5:-"TestSite"}
LICENSE=\${6:-"xxxx-xxxx-xxxx-xxxx-xxxx-xxxx"}
HOSTID=\${7:-0}
RUNID=\${8:-0}

if [[ ! -f "\$JOURNAL" ]]; then
    echo "ERROR: Journal file not found: \$JOURNAL"
    exit 1
fi

echo "Starting encrypted replay..."
echo "  Journal: \$JOURNAL"
echo "  Site Name: \$SITENAME"
echo "  Site ID: \$SITEID"
echo ""

perfreplay --siteid=\$SITEID --sitename="\$SITENAME" --license="\$LICENSE" \\
    --host=\$HOSTID --run=\$RUNID \\
    --input="\$JOURNAL" \\
    --training="\$TRAINING" \\
    --diskmapper="\$DISKMAPPER" \\
    --log=prp=INFO
EOF
chmod +x "$PERFCOLLECTOR_HOME/bin/run-replay-encrypted.sh"

echo "[5/5] Setup complete!"
echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Copy PerfCollector binaries to: $PERFCOLLECTOR_HOME/bin/"
echo "2. Add the following to your PATH:"
echo "   export PATH=\$PATH:$PERFCOLLECTOR_HOME/bin"
echo ""
echo "3. Run CPU training (required before replay):"
echo "   $PERFCOLLECTOR_HOME/bin/run-cpu-training.sh"
echo ""
echo "4. Copy journal file from Azure VM to: $REPLAY_DATA_DIR/journal"
echo ""
echo "5. Run replay:"
echo "   $PERFCOLLECTOR_HOME/bin/run-replay.sh $REPLAY_DATA_DIR/journal"
echo ""
echo "Disk replay directories created at: $DISK_REPLAY_DIR"
echo "Edit $REPLAY_DATA_DIR/diskmapper.json to match actual device names"
EOF
