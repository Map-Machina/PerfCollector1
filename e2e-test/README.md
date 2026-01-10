# End-to-End Test: Azure to OCI Performance Replay

This test validates the complete PerfCollector workflow by capturing performance data during a sysbench benchmark on Azure and replaying it on OCI.

## Quick Start

### Prerequisites

1. SSH access to both VMs
2. PerfCollector binaries built for Linux
3. sysbench installed on Azure VM

### Run the Full Test

```bash
# From your local machine
./06-run-e2e-test.sh \
    --azure-vm pcc-test-01 \
    --oci-vm perftest-vm-02 \
    --duration 300 \
    --auto
```

## Test Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        LOCAL MACHINE                             │
│                    (Test Orchestrator)                           │
│                                                                   │
│     06-run-e2e-test.sh                                          │
│         │                                                         │
│         ├── SSH ──► Azure VM (pcc-test-01)                      │
│         │              │                                          │
│         │              ├── Run sysbench benchmark                │
│         │              └── Capture /proc metrics                 │
│         │                                                         │
│         └── SSH ──► OCI VM (perftest-vm-02)                     │
│                        │                                          │
│                        ├── Run CPU training                      │
│                        ├── Replay captured workload              │
│                        └── Validate results                      │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

## Scripts

| Script | Purpose | Run On |
|--------|---------|--------|
| `01-setup-azure.sh` | Setup Azure VM for capture | Azure VM |
| `02-setup-oci.sh` | Setup OCI VM for replay | OCI VM |
| `03-run-benchmark.sh` | Run sysbench workloads | Azure VM |
| `04-capture-simple.sh` | Capture /proc metrics | Azure VM |
| `05-validate-replay.sh` | Run replay and validate | OCI VM |
| `06-run-e2e-test.sh` | Orchestrate full test | Local |

## Manual Execution Steps

### Step 1: Setup Azure VM (pcc-test-01)

```bash
# SSH to Azure VM
ssh azureuser@pcc-test-01

# Run setup
./01-setup-azure.sh

# Copy PerfCollector binaries
# (build with: GOOS=linux go build ./cmd/...)
```

### Step 2: Setup OCI VM (perftest-vm-02)

```bash
# SSH to OCI VM
ssh opc@perftest-vm-02

# Run setup
./02-setup-oci.sh

# Copy PerfCollector binaries
```

### Step 3: Capture on Azure VM

```bash
# Start capture in background
./04-capture-simple.sh &

# Run benchmark
./03-run-benchmark.sh

# Wait for capture to finish
```

### Step 4: Transfer Data

```bash
# From local machine
scp azureuser@pcc-test-01:~/benchmark-capture.json /tmp/
scp /tmp/benchmark-capture.json opc@perftest-vm-02:~/replay-data/journal
```

### Step 5: Train on OCI VM

```bash
# SSH to OCI VM
ssh opc@perftest-vm-02

# Run CPU training
cd ~/replay-data
perfcpumeasure --siteid=1 --host=0 -v > training.json
```

### Step 6: Replay and Validate

```bash
# On OCI VM
./05-validate-replay.sh
```

## Sysbench Workloads

The benchmark runs these workloads:

1. **CPU-Intensive** (5 min): Prime number computation
2. **Memory-Intensive** (varies): Sequential memory writes
3. **Mixed CPU+Memory** (5 min): Concurrent CPU and memory load
4. **Variable Load** (2 min): Ramping load pattern (25%→100%→25%)

## Validation Criteria

| Metric | Target | Description |
|--------|--------|-------------|
| RMSE | < 5% | Root Mean Square Error for CPU |
| Correlation | > 0.95 | Pattern similarity |
| Within ±5% | > 80% | Samples close to original |
| Within ±10% | > 95% | Samples within tolerance |

## Troubleshooting

### Connection Issues

```bash
# Test Azure connectivity
ssh -v azureuser@pcc-test-01 echo "OK"

# Test OCI connectivity
ssh -v opc@perftest-vm-02 echo "OK"
```

### Missing Binaries

```bash
# Build on local machine
cd /path/to/perfcollector
GOOS=linux GOARCH=amd64 go build -o perfcpumeasure ./cmd/perfcpumeasure
GOOS=linux GOARCH=amd64 go build -o perfreplay ./cmd/perfreplay

# Copy to VM
scp perfcpumeasure perfreplay opc@perftest-vm-02:~/perfcollector/bin/
```

### Replay Errors

Check training data:
```bash
cat ~/replay-data/training.json
```

Check journal format:
```bash
head -5 ~/replay-data/journal | jq .
```

## Output Files

After successful test:

```
results_YYYYMMDD_HHMMSS/
├── original_capture.json     # Captured metrics from Azure
├── training.json             # CPU calibration data from OCI
├── replay_metrics_*.csv      # Metrics during replay
├── replay_log.txt            # Replay stdout/stderr
├── validation_report_*.txt   # Validation summary
└── cpu_intensive_*.log       # Sysbench output
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AZURE_USER` | azureuser | Azure VM SSH user |
| `OCI_USER` | opc | OCI VM SSH user |
| `SSH_KEY` | ~/.ssh/id_rsa | SSH private key path |
| `BENCHMARK_DURATION` | 300 | Test duration in seconds |
| `COLLECTION_INTERVAL` | 5 | Metrics collection interval |
