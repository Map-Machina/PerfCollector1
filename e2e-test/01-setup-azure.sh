#!/bin/bash
# Setup script for Azure VM (pcc-test-01) - Source/Capture system
# This script prepares the Azure VM for performance data collection

set -e

# Configuration
PERFCOLLECTOR_HOME="${PERFCOLLECTOR_HOME:-$HOME/perfcollector}"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/id_ed25519}"
LISTEN_PORT="${LISTEN_PORT:-2222}"

echo "=== PerfCollector Azure VM Setup (pcc-test-01) ==="
echo "Configuration:"
echo "  PERFCOLLECTOR_HOME: $PERFCOLLECTOR_HOME"
echo "  SSH_KEY_PATH: $SSH_KEY_PATH"
echo "  LISTEN_PORT: $LISTEN_PORT"
echo ""

# Check if running on Linux
if [[ "$(uname)" != "Linux" ]]; then
    echo "ERROR: This script must be run on Linux"
    exit 1
fi

# Create directories
echo "[1/6] Creating directories..."
mkdir -p "$PERFCOLLECTOR_HOME"/{bin,config,data}
mkdir -p "$HOME/.perfcollectord"
mkdir -p "$HOME/.perfprocessord/data"

# Install sysbench if not present
echo "[2/6] Checking sysbench installation..."
if ! command -v sysbench &> /dev/null; then
    echo "Installing sysbench..."
    if command -v apt-get &> /dev/null; then
        sudo apt-get update && sudo apt-get install -y sysbench
    elif command -v yum &> /dev/null; then
        sudo yum install -y epel-release
        sudo yum install -y sysbench
    elif command -v dnf &> /dev/null; then
        sudo dnf install -y sysbench
    else
        echo "WARNING: Could not install sysbench automatically"
        echo "Please install sysbench manually"
    fi
else
    echo "sysbench already installed: $(sysbench --version)"
fi

# Generate SSH key if not present
echo "[3/6] Checking SSH key..."
if [[ ! -f "$SSH_KEY_PATH" ]]; then
    echo "Generating SSH key..."
    ssh-keygen -t ed25519 -f "$SSH_KEY_PATH" -N ""
fi

# Get SSH fingerprint
SSH_FINGERPRINT=$(ssh-keygen -l -f "$SSH_KEY_PATH" -E SHA256 | awk '{print $2}')
echo "SSH Fingerprint: $SSH_FINGERPRINT"

# Check if binaries exist
echo "[4/6] Checking PerfCollector binaries..."
BINARIES_NEEDED="perfcollectord perfprocessord"
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
    echo "  GOOS=linux GOARCH=amd64 go build -o perfcollectord ./cmd/perfcollectord"
    echo "  GOOS=linux GOARCH=amd64 go build -o perfprocessord ./cmd/perfprocessord"
fi

# Create perfcollectord config
echo "[5/6] Creating perfcollectord configuration..."
cat > "$HOME/.perfcollectord/perfcollectord.conf" << EOF
# PerfCollector Daemon Configuration
# Azure VM: pcc-test-01

# SSH key for authentication
sshid=$SSH_KEY_PATH

# Listen address (all interfaces)
listen=0.0.0.0:$LISTEN_PORT

# Allowed SSH key fingerprints (add perfprocessord's key fingerprint here)
# allowedkeys=$SSH_FINGERPRINT

# Debug logging (uncomment for troubleshooting)
# log=pcd=DEBUG
EOF

echo "Configuration written to: $HOME/.perfcollectord/perfcollectord.conf"

# Create helper scripts
echo "[6/6] Creating helper scripts..."

# Start collector script
cat > "$PERFCOLLECTOR_HOME/bin/start-collector.sh" << 'EOF'
#!/bin/bash
# Start perfcollectord daemon
cd $HOME
nohup perfcollectord > /tmp/perfcollectord.log 2>&1 &
echo "perfcollectord started (PID: $!)"
echo "Log: /tmp/perfcollectord.log"
EOF
chmod +x "$PERFCOLLECTOR_HOME/bin/start-collector.sh"

# Stop collector script
cat > "$PERFCOLLECTOR_HOME/bin/stop-collector.sh" << 'EOF'
#!/bin/bash
# Stop perfcollectord daemon
pkill -f perfcollectord
echo "perfcollectord stopped"
EOF
chmod +x "$PERFCOLLECTOR_HOME/bin/stop-collector.sh"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Copy PerfCollector binaries to: $PERFCOLLECTOR_HOME/bin/"
echo "2. Add the following to your PATH:"
echo "   export PATH=\$PATH:$PERFCOLLECTOR_HOME/bin"
echo ""
echo "3. Share this SSH fingerprint with perfprocessord:"
echo "   $SSH_FINGERPRINT"
echo ""
echo "4. Start the collector:"
echo "   $PERFCOLLECTOR_HOME/bin/start-collector.sh"
echo ""
echo "5. Verify it's running:"
echo "   ps aux | grep perfcollectord"
echo "   netstat -tlnp | grep $LISTEN_PORT"
EOF
