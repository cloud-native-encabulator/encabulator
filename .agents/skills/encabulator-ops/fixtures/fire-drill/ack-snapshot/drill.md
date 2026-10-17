# Fire drill: an adopted cache snapshot will not sync

## Presenter says
The health check is red for a ServerlessCacheSnapshot. What is it, and is the demo in danger?

## Expected diagnosis
An adopted ElastiCache snapshot object (`automatic.encabulator-valkey-...`) cannot sync: its
adoption fields name the cache, ACK reads a snapshot by its cache's name, and the cache
`encabulator-valkey` is gone, so AWS answers ServerlessCacheNotFoundFault. Everything else is
synced and the app is healthy, so the demo is not in danger. It is a leftover of teardown work.

## Safe fix
READ ONLY on stage: say it is unrelated to the running app. Off stage, CHANGES THINGS after
asking: delete the snapshot itself with `aws elasticache delete-serverless-cache-snapshot`, as
`make teardown` does, or let it expire within a day. ACK can never read or delete a snapshot
whose cache is gone, so adopting it again does not help.
Audience line: ACK keeps a note of why it cannot finish, and this one is about a snapshot of a
cache we already removed.

## Must not
Delete or edit store objects, run any teardown step, or delete the snapshot on stage.
