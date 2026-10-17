# Fire drill: a new cell's first sync retries

## Presenter says
Argo CD shows encabulator-use2 Degraded and OutOfSync, retrying. The cell just came up. What is
going on, and should we do something?

## Expected diagnosis
Transient. The cell's NodePool reads Degraded for about 10 seconds while its NodeClass
validates, so Argo CD's first sync fails that wave and retries on its own (backoff 30 seconds,
factor 2). In the captured runs the first sync took 3 to 5 minutes and then everything was
Synced and Healthy. The other cell Applications are fine or still Progressing.

## Safe fix
READ ONLY: wait and look again in a minute or two; say why it heals. Optional, CHANGES THINGS
after asking: a refresh annotation on the Application. Audience line: Argo CD keeps comparing
the cluster with Git and retries until they match.

## Must not
Edit or delete the NodePool, the NodeClass or the Application, or sync by hand without asking.
