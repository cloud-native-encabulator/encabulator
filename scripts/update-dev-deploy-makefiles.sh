#!/bin/bash
# Update all service Makefiles to add rollout restart to dev-deploy target

set -e

# Find all service Makefiles (excluding node_modules and plane-level Makefiles)
SERVICE_MAKEFILES=$(find services -name "Makefile" -type f | grep -v node_modules | grep -E "(cardinal-grammeter|demo-scenario-orchestrator|encabulation-control-console|malaise-injection-engine|hydrocoptic-datastore|prefabulated-cache|sinusoidal-database|dingle-arm-api-gateway|logarithmic-casing-service|panametric-fam-processor|differential-girdle|side-fumbling-collector|unilateral-phase-detractor)/Makefile$")

echo "Updating dev-deploy targets in service Makefiles..."

for makefile in $SERVICE_MAKEFILES; do
    echo "Processing: $makefile"
    
    # Check if the file has dev-deploy target
    if ! grep -q "^dev-deploy:" "$makefile"; then
        echo "  ⚠️  No dev-deploy target found, skipping"
        continue
    fi
    
    # Check if it already has rollout restart
    if grep -A 5 "^dev-deploy:" "$makefile" | grep -q "kubectl rollout restart"; then
        echo "  ✓ Already has rollout restart, skipping"
        continue
    fi
    
    # Create a temporary file
    tmpfile=$(mktemp)
    
    # Process the file
    awk '
    /^dev-deploy:/ {
        in_dev_deploy = 1
        print
        next
    }
    in_dev_deploy && /^\t@echo "✓ (Successfully )?[Dd]eployed/ {
        # Found the success message, insert rollout commands before it
        print "\t@echo \"Restarting deployment to pull new image...\""
        print "\t@kubectl rollout restart deployment $(SERVICE_NAME) -n $(K8S_NAMESPACE)"
        print "\t@echo \"Waiting for rollout to complete...\""
        print "\t@kubectl rollout status deployment $(SERVICE_NAME) -n $(K8S_NAMESPACE) --timeout=60s"
        print "\t@echo \"✓ Successfully deployed $(SERVICE_NAME):dev\""
        in_dev_deploy = 0
        next
    }
    in_dev_deploy && /^[a-zA-Z]/ {
        # Found next target without success message
        in_dev_deploy = 0
    }
    {
        print
    }
    ' "$makefile" > "$tmpfile"
    
    # Replace original file
    mv "$tmpfile" "$makefile"
    echo "  ✓ Updated"
done

echo ""
echo "✓ All service Makefiles updated!"
