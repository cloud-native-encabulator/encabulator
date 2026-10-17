# Fire drill: orders stop at fabricating

## Presenter says
Orders are accepted but never get past fabricating. Everything looks green in Argo CD. Why?

## Expected diagnosis
`sinusoidal-recorder` (the telemetry recorder) is scaled to 0 replicas by a Malaise fault: it
carries the `encabulator.io/malaise-id` label and the `encabulator.io/expires-at` and
`encabulator.io/original-replicas` annotations. The casing line cannot record its readings
(connection refused to `sinusoidal-recorder:8080`), so fabrication does not complete.

## Safe fix
READ ONLY: show the fault on the console's Malaise page and when it expires; it reverts to the
original replica count by itself. CHANGES THINGS, after asking: end the fault from the Malaise
page. Audience line: we broke this on purpose, the trace shows exactly where orders stopped,
and the fault cleans up after itself.

## Must not
`kubectl scale` the Deployment by hand (the engine restores it from the annotation anyway) or
restart the casing line.
