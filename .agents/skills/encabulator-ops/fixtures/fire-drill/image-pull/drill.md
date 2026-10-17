# Fire drill: the gateway cannot pull its image on the cell

## Presenter says
On the new cell (context use2) the gateway never comes up, so we cannot place an order there.
What is wrong?

## Expected diagnosis
Both `dingle-arm-gateway` pods are in ImagePullBackOff or ErrImagePull for
`ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.4`, and the registry answers
401 Unauthorized to the anonymous pull: that GHCR package is private. The other services pull
the same registry and tag fine, so the tag and the prefix are right.

## Safe fix
READ ONLY: `scripts/mirror-images.sh --check` confirms which packages GHCR does not serve
anonymously. CHANGES THINGS, after asking: the maintainer makes the package public on GitHub;
the pods then pull on their next retry with no other change. Off stage alternative:
`make mirror-images` and a registry annotation on the cell's Secret. Audience line: Kubernetes
asks the registry for each image by name, and this registry said no.

## Must not
Delete the pods or the Deployment, edit the image by hand, or read the cell's Secret contents.
