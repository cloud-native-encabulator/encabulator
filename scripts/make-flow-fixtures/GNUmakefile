# Used only by scripts/make-flow_test.sh, which copies it next to a copy of the Makefile.
# GNU make reads GNUmakefile before Makefile, so every make in the test (sub-makes included)
# gets these stand-ins for the targets the test does not exercise: the platform charts and
# the GitOps steps under gitops/. Each records that it ran and what it was handed.
include Makefile

platform gitops-register gitops-repo-access gitops-bootstrap:
	@echo "target $@ | REGISTRY=$${REGISTRY:-} GIT_REPO_URL=$${GIT_REPO_URL:-} GIT_REVISION=$${GIT_REVISION:-} GIT_TOKEN_FILE=$${GIT_TOKEN_FILE:-} AWS_REGION=$${AWS_REGION:-}" >>"$(STUB_LOG)"

# The settings a recipe sees in its environment.
print-env:
	@env | grep -E '^(IDC_INSTANCE_ARN|IDC_REGION|ARGOCD_ADMIN_GROUP_ID|GIT_REPO_URL|GIT_REVISION|GIT_TOKEN_FILE|REGISTRY|CONTAINER_RUNTIME|TF_VAR_[a-z_]+)=' | sort || true
