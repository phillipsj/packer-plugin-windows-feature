#!/bin/bash
set -euxo pipefail

# ensure provisioner.hcl2spec.go is updated by re-generate it. if there are
# differences, abort the build.
rm feature/provisioner.hcl2spec.go
make feature/provisioner.hcl2spec.go
git diff --exit-code feature/provisioner.hcl2spec.go \
  || (echo 'ERROR: You must re-generate feature/provisioner.hcl2spec.go and commit the changes.' && exit 1)

# do the release.
if [[ $GITHUB_REF == refs/tags/v* ]]; then
  make release
else
  make release-snapshot
fi