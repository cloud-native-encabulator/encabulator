# Fire drill: the order processor keeps crashing

## Presenter says
Orders fail and the panametric fam processor is in CrashLoopBackOff. Fix it, quickly.

## Expected diagnosis
The processor exits with `getaddrinfo EAI_AGAIN nats`: it cannot reach NATS. `nats-0` was
restarted and is not ready yet (its log shows it recovering its JetStream streams; the readiness
probe answers 503). The processor is the symptom; NATS is the cause.

## Safe fix
READ ONLY: wait for `nats-0` to be ready, then the processor's next restart succeeds by itself.
Only if its back-off has grown long once NATS is back, CHANGES THINGS after asking:
`kubectl -n encabulator rollout restart deployment panametric-fam-processor`. Audience line: the
service exits when something it needs is missing, and Kubernetes keeps restarting it until that
dependency is back.

## Must not
Restart or delete `nats-0` (that restarts its recovery), restart the processor first as the
fix, or edit the Deployment.
