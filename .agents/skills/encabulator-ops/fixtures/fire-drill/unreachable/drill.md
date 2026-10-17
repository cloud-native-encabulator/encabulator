# Fire drill: the cluster does not answer

## Presenter says
kubectl suddenly says I must be logged in. We are live. What now?

## Expected diagnosis
kubectl cannot authenticate to the hub (it asks for credentials) and the AWS CLI's token has
expired (ExpiredToken). The cluster itself is probably fine; the laptop's credentials are not.

## Safe fix
On stage: one line saying so, then continue with the captured runs: the deck's captured run
("Where the time goes"), the Act 3 timings in `docs/opn315/RUN-OF-SHOW.md`, the measured table
in the README. Off stage: refresh the AWS credentials, then `make kubeconfig`. Audience line:
the cluster is fine, the laptop lost its credentials; here is the same step from a captured run.

## Must not
Debug credentials on the shared screen, print a token or a credentials file, or change
kubeconfig contexts at random.
