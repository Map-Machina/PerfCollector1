# End-to-End Test Plan: Azure to OCI Performance Replay

## Overview

This test validates the complete PerfCollector workflow by:
1. Running a sysbench benchmark on Azure VM (pcc-test-01)
2. Capturing performance metrics during the benchmark
3. Replaying the captured workload on OCI VM (perftest-vm-02)
4. Validating that the replay accurately reproduces the original load patterns

## Test Environment

| Role | VM Name | Cloud | Purpose |
|------|---------|-------|---------|
| Source (Capture) | pcc-test-01 | Azure | Run sysbench, capture metrics |
| Target (Replay) | perftest-vm-02 | OCI | CPU training, workload replay |

## Prerequisites

### Both VMs
- Go 1.21+ installed
- PerfCollector binaries built and deployed
- SSH key pair generated
- Root or sudo access for system monitoring

### Azure VM (pcc-test-01)
- sysbench installed (`apt install sysbench` or `yum install sysbench`)
- perfcollectord binary
- SSH key for perfprocessord connection

### OCI VM (perftest-vm-02)
- perfcpumeasure binary
- perfreplay binary
- Disk mount points for I/O replay (optional)

## Test Phases

### Phase 1: Environment Setup

1. **Build and deploy binaries**
2. **Configure SSH connectivity**
3. **Install sysbench on Azure VM**
4. **Create disk mapper configuration** (if testing disk I/O)

### Phase 2: Capture (Azure VM)

1. **Start perfcollectord** on pcc-test-01
2. **Start perfprocessord** (can run on same VM or separate)
3. **Begin collection**
4. **Run sysbench benchmark** (CPU, memory, and optionally I/O)
5. **Stop collection**
6. **Transfer journal file** to OCI VM

### Phase 3: Training (OCI VM)

1. **Run perfcpumeasure** to generate CPU training data
2. **Create disk mapper** configuration (if testing I/O)

### Phase 4: Replay (OCI VM)

1. **Run perfreplay** with training data and journal
2. **Capture metrics** during replay for comparison
3. **Validate results** using validation framework

### Phase 5: Validation

1. **Compare original vs replay metrics**
2. **Check acceptance criteria**:
   - RMSE < 5%
   - 80% of samples within ±5%
   - 95% of samples within ±10%
   - Correlation coefficient > 0.95

## Sysbench Workload Profiles

### CPU-Intensive Test
```bash
sysbench cpu --cpu-max-prime=20000 --threads=4 --time=300 run
```

### Memory-Intensive Test
```bash
sysbench memory --memory-block-size=1M --memory-total-size=10G --threads=4 run
```

### Mixed Workload Test
```bash
# Run CPU and memory tests concurrently
sysbench cpu --cpu-max-prime=10000 --threads=2 --time=300 run &
sysbench memory --memory-block-size=512K --memory-total-size=5G --threads=2 run &
wait
```

### I/O Test (Optional)
```bash
# Prepare test files
sysbench fileio --file-total-size=2G prepare

# Run I/O test
sysbench fileio --file-total-size=2G --file-test-mode=rndrw --time=300 run

# Cleanup
sysbench fileio --file-total-size=2G cleanup
```

## Success Criteria

| Metric | Threshold | Description |
|--------|-----------|-------------|
| CPU RMSE | < 5% | Root mean square error for CPU utilization |
| CPU Correlation | > 0.95 | Pearson correlation coefficient |
| Memory Pattern Match | > 80% | Samples within ±10% of original |
| Test Completion | 100% | All phases complete without errors |

## Files Generated

| File | Location | Description |
|------|----------|-------------|
| journal | ~/.perfprocessord/data/journal | Encrypted performance data |
| training.json | ~/training.json | CPU calibration data |
| diskmapper.json | ~/diskmapper.json | Disk mapping configuration |
| validation_report.csv | ~/validation_report.csv | Validation results |

## Rollback Procedures

If any phase fails:
1. Stop all running processes (`pkill -f perf`)
2. Collect logs for analysis
3. Clean up test files
4. Document failure point and error messages

## Estimated Duration

| Phase | Duration |
|-------|----------|
| Setup | 15 minutes |
| Capture | 10-15 minutes (5 min benchmark + overhead) |
| Training | 2-3 minutes |
| Replay | Same as benchmark duration |
| Validation | 5 minutes |
| **Total** | **~45 minutes** |
