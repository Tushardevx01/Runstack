# Troubleshooting RunStack

## General Issues

### 1. Address Already in Use (Control Plane)
```text
bind: address already in use
```
**Cause:** An orphaned Control Plane process is still running from a previous crash or integration test.
**Solution:** Find the PID using `lsof -i :8080` and `kill -9 <PID>`, or just run `pkill control-plane`.

### 2. Node Stuck Offline / Jobs Stranded
**Cause:** The agent crashed, network partitioning occurred, or you manually stopped the agent. 
**Solution:** In V1, stranded jobs are not automatically rescheduled. If you bring the agent back up, it will resume heartbeats and become `ONLINE`, and then the scheduler will push new jobs to it, and it will eventually pick up its stranded `PENDING` assignments.

### 3. Agent Failing to Register
```text
Registration failed: connection refused. Retrying in 5 seconds...
```
**Cause:** The Control Plane is not running or is listening on a different port/interface.
**Solution:** Start the Control Plane (`make control-plane`) and ensure it's binding to `8080`.

### 4. Integration Tests Hanging or Failing Deterministically
**Cause:** 
- The offline detector loop takes 30 seconds to trigger. If scripts don't `sleep 35`, assertions will fail.
- The scheduler loop takes 5 seconds to trigger.
- The agent polling loop takes 3 seconds to trigger.
**Solution:** Pad your sleep times in integration shell scripts carefully to accommodate background Go routines.

### 5. `exec: "echo \"hello": executable file not found in $PATH`
**Cause:** V1 command parsing uses `strings.Fields`, which splits blindly by spaces. Quoted strings are not interpreted as a single argument natively by `os/exec`.
**Solution:** Avoid shell quotes when creating jobs in V1, or inject a wrapper. Example: instead of `echo "hello world"`, rely on single unspaced arguments, or await V2 command shell enhancements.
