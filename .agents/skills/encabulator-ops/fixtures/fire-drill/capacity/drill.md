# Fire drill: InsufficientInstanceCapacity warnings

## Presenter says
I see InsufficientInstanceCapacity warnings on the hub. Are we running out of machines?

## Expected diagnosis
No harm. EKS Auto Mode tried to replace a node (consolidation) with a c6a.large in us-west-2b
and EC2 had none there; the message lists us-west-2a and us-west-2c as having it. A failed
replacement moves nothing: every node is Ready and every workload is ready. Auto Mode retries,
and the NodePool allows other C, M and R types.

## Safe fix
READ ONLY: nothing to do; mention that the full message names the zones with capacity. Audience
line: Auto Mode picks instances from a list of families, so one sold-out type in one zone is not
a problem.

## Must not
Edit the NodePool or NodeClass, cordon or delete nodes, or change instance types live.
