# Fire drill: the cell never reaches ACTIVE

## Presenter says
The new region's cluster is ACTIVE, but the EncabulatorCell has been IN_PROGRESS for a long
time and no cell Application appears in Argo CD. What is it waiting for?

## Expected diagnosis
The cell's graph waits for "external reference hub": the hub's Argo CD cluster Secret
`encabulator-usw2` does not exist, because the hub was never registered with its own Argo CD
(`make gitops-register`). kro has built the cell's AWS resources, but it cannot write the cell's
Argo CD Secret without the hub's settings, so Argo CD deploys nothing to the cell.

## Safe fix
CHANGES THINGS, after asking: `make gitops-register` (it writes the hub's cluster Secret; a
`REGISTER_ARGS=--dry-run` run shows it first). The cell then goes ACTIVE within seconds and its
Applications appear. Audience line: kro builds each piece once what it needs exists, and this
piece needs the hub's registration.

## Must not
Delete or recreate the EncabulatorCell, or edit kro's objects by hand.
