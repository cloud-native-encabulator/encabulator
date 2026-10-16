# shellcheck shell=bash
# What the scripts that run Terraform say when Terraform itself fails. Sourced by
# scripts/kubeconfig.sh, scripts/prereqs.sh and scripts/teardown.sh.
#
# A version manager's shim fails before Terraform runs when no version is set for the
# directory, and its message names that ("mise ERROR No version is set for shim: terraform").
# Its last line is only "Run with --verbose ...", so the line passed on is the first one that
# says error, and the fix is the TERRAFORM setting.

# terraform_error [FILE]: the line of FILE (or stdin), a command's error output, that names the
# error: the first line that says error, else the last line that is not empty. Terraform's
# frame (the box-drawing characters at the start of each line) and its colors are left out.
terraform_error() {
  sed -E -e $'s/\033\\[[0-9;]*m//g' -e 's/^(│|╷|╵) ?//' -e 's/^[[:space:]]+//' "$@" \
    | awk 'NF && hit == "" && tolower($0) ~ /error/ { hit = $0 }
           NF { last = $0 }
           END { print (hit != "" ? hit : last) }'
}

# terraform_hint: the fix when Terraform comes from a version manager. Each script adds which
# command to run again.
terraform_hint() {
  printf '%s' 'if Terraform comes from a version manager, export TERRAFORM="mise exec terraform@1.14.9 -- terraform" (it is read from the environment or the make command line, never from .env)'
}
