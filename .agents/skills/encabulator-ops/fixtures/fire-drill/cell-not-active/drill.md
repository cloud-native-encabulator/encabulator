# Fire drill: ten minutes in, the cell is not ACTIVE

## Presenter says
We pushed the new region ten minutes ago. The cell still says IN_PROGRESS and the cluster says
CREATING. Is it broken?

## Expected diagnosis
Not broken; on schedule. In the captured runs the cell's cluster is ACTIVE about 13 minutes
after the trigger and the cell about 20 minutes; here the network, the NAT gateway, the route
table and the roles are already synced, and the cluster is still CREATING, which is what kro
waits for next.

## Safe fix
READ ONLY: give the timeline (cluster about 13 minutes, cell about 20, app Healthy about 32) and
look again later. If the talk reaches Act 3 before the cell is Healthy, switch to the prepared
fallback cell and say so. Audience line: building a region is mostly waiting for AWS, and kro
starts each piece the moment the one before it is ready.

## Must not
Delete or re-push the cell, or touch the ACK objects.
