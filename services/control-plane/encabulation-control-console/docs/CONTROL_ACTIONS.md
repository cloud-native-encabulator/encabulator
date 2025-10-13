# Control Actions

The Encabulation Control Console provides seven themed control actions for managing and testing the Cloud Native Encabulator system. These actions are available to authenticated users (admin and operator roles) through the Actions Panel.

## Available Actions

### 1. Synchronize Cardinal Grammeters
**Purpose**: Initiate load testing and traffic generation

**What it does**:
- Triggers the Cardinal Grammeter service to start generating synthetic traffic
- Targets the Dingle Arm API Gateway with steady traffic pattern
- Configures magnetoReluctance and capacitiveDiractance parameters
- Useful for testing system performance under load

**API Endpoint**: `POST /api/actions/synchronize-grammeters`

**Use Cases**:
- Performance testing
- Stress testing the API gateway
- Validating auto-scaling behavior
- Demo scenarios

---

### 2. Induce Side Fumbling
**Purpose**: Inject controlled chaos for resilience testing

**What it does**:
- Activates the Malaise Injection Engine to inject latency
- Targets the Panametric Fam Processor service
- Runs for 5 minutes (300 seconds) by default
- Tests system resilience and error handling

**API Endpoint**: `POST /api/actions/induce-fumbling`

**Use Cases**:
- Chaos engineering experiments
- Testing circuit breakers and retries
- Validating graceful degradation
- Resilience demonstrations

---

### 3. Flex Dingle Arm (Scale Up)
**Purpose**: Increase capacity of the API Gateway

**What it does**:
- Scales the Dingle Arm API Gateway deployment up by 1 replica
- Uses Kubernetes API to modify deployment replicas
- Demonstrates horizontal scaling capabilities
- Increases request handling capacity

**API Endpoint**: `POST /api/actions/flex-dingle-arm` with `{"direction": "up"}`

**Use Cases**:
- Handling increased traffic
- Demonstrating auto-scaling
- Capacity planning
- Load distribution testing

---

### 4. Flex Dingle Arm (Scale Down)
**Purpose**: Reduce capacity of the API Gateway

**What it does**:
- Scales the Dingle Arm API Gateway deployment down by 1 replica
- Minimum of 1 replica is maintained
- Demonstrates scale-down behavior
- Reduces resource consumption

**API Endpoint**: `POST /api/actions/flex-dingle-arm` with `{"direction": "down"}`

**Use Cases**:
- Cost optimization demonstrations
- Testing scale-down behavior
- Resource management
- Validating graceful pod termination

---

### 5. Recalibrate Panametric Fam
**Purpose**: Restart the core processing service

**What it does**:
- Performs a rolling restart of the Panametric Fam Processor
- Updates deployment with restart annotation
- Kubernetes performs graceful pod replacement
- Tests deployment resilience and zero-downtime updates

**API Endpoint**: `POST /api/actions/recalibrate-fam`

**Use Cases**:
- Simulating service updates
- Testing rolling restart behavior
- Demonstrating zero-downtime deployments
- Recovering from stuck states

---

### 6. Engage Malaise Dampeners
**Purpose**: Stop all traffic generation and chaos experiments

**What it does**:
- Stops the Cardinal Grammeter traffic generation
- Halts any active chaos experiments
- Returns system to steady state
- Cleans up test scenarios

**API Endpoint**: `POST /api/actions/engage-dampeners`

**Use Cases**:
- Ending demo scenarios
- Stopping load tests
- Returning to baseline
- Emergency stop for experiments

---

### 7. Purge Hydrocoptic Marzelvanes
**Purpose**: Clear the distributed cache

**What it does**:
- Clears all data from the Prefabulated Cache (Redis)
- Forces cache refresh on next requests
- Tests cache invalidation behavior
- Demonstrates cache management

**API Endpoint**: `POST /api/actions/purge-marzelvanes`

**Use Cases**:
- Cache invalidation testing
- Demonstrating cache behavior
- Performance testing (cold vs warm cache)
- Troubleshooting stale data

---

## Action History

The Actions Panel includes an action history log that displays:
- Last 10 actions performed
- Success/failure status (green/red indicator)
- Action message
- Timestamp

This provides visibility into recent control operations and their outcomes.

---

## Access Control

### Guest Users
- **Cannot** perform control actions
- See informative message with lock icon
- Prompted to sign in for full access
- Can view all system status and metrics

### Authenticated Users
- **Admin**: Full access to all control actions
- **Operator**: Full access to all control actions
- **Viewer**: Read-only access (no control actions)

---

## Safety Considerations

### Production Use
These control actions are designed for:
- **Development environments**: Full testing and experimentation
- **Staging environments**: Pre-production validation
- **Demo environments**: Customer demonstrations

For production environments:
- Implement additional authorization checks
- Add confirmation dialogs for destructive actions
- Configure rate limiting
- Enable audit logging
- Restrict to admin users only

### Rate Limiting
Consider implementing rate limiting on control actions to prevent:
- Accidental rapid-fire actions
- Resource exhaustion
- Cascading failures

### Audit Trail
All control actions should be logged for:
- Compliance requirements
- Troubleshooting
- Security auditing
- Change tracking

---

## Technical Implementation

### Backend
Control actions are implemented as REST API endpoints in the Express server:
- Located in `server/src/index.ts`
- Use Kubernetes client for cluster operations
- Call external service APIs (Cardinal Grammeter, Malaise Injection Engine, etc.)
- Return standardized response format

### Frontend
Actions Panel component (`src/components/ActionsPanel.vue`):
- Gradient-styled themed buttons
- Loading states during operations
- Action history with success/failure tracking
- Guest access restriction
- Error handling and user feedback

### Response Format
```json
{
  "success": true,
  "message": "Action completed successfully",
  "data": { /* optional action-specific data */ },
  "timestamp": "2025-10-12T16:00:00.000Z"
}
```

---

## Future Enhancements

Potential improvements for control actions:
- [ ] Confirmation dialogs for destructive actions
- [ ] Action scheduling (run at specific time)
- [ ] Action chaining (run multiple actions in sequence)
- [ ] Custom action parameters (duration, intensity, etc.)
- [ ] Action templates (save common action sequences)
- [ ] Rollback capability for reversible actions
- [ ] Action impact preview (show what will happen)
- [ ] Bulk actions (perform on multiple services)

---

## Troubleshooting

### Action Fails with "Service not found"
- Verify the target service is deployed
- Check service name matches deployment name
- Ensure service is in expected namespace
- Verify RBAC permissions for service account

### Action Succeeds but No Effect
- Check backend service logs
- Verify target service API is responding
- Ensure Kubernetes API connectivity
- Check for conflicting HPA or other controllers

### Actions Disabled/Hidden
- Verify user is authenticated (not guest)
- Check user role (admin/operator required)
- Ensure frontend is connected to backend
- Check browser console for errors

---

## Related Documentation

- [Guest Access](../GUEST-ACCESS.md) - Guest user capabilities and restrictions
- [Traffic Metrics](./TRAFFIC_METRICS.md) - Traffic visualization and metrics
- [Particle Colors](./PARTICLE_COLORS.md) - Traffic health color coding
- [README](../README.md) - Main console documentation
