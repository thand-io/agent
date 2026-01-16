#!/bin/bash

###############################################################################
# Azure Container App Permissions Verification Script
#
# This script verifies that all necessary permissions have been assigned
# and admin consent has been granted for a Container App's managed identity.
#
# Usage:
#   ./azure-verify-permissions.sh \
#     --container-app <container-app-name> \
#     --resource-group <resource-group> \
#     --subscription-id <subscription-id>
#
###############################################################################

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
CONTAINER_APP_NAME=""
RESOURCE_GROUP=""
SUBSCRIPTION_ID=""

# Function to print colored messages
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

# Function to display usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Verifies Azure permissions for a Container App's managed identity.

Required Options:
  --container-app NAME       Name of the Container App
  --resource-group NAME      Resource Group containing the Container App
  --subscription-id ID       Azure Subscription ID

Examples:
  $0 --container-app my-agent --resource-group my-rg --subscription-id 12345678-1234-1234-1234-123456789abc

EOF
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --container-app)
            CONTAINER_APP_NAME="$2"
            shift 2
            ;;
        --resource-group)
            RESOURCE_GROUP="$2"
            shift 2
            ;;
        --subscription-id)
            SUBSCRIPTION_ID="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            ;;
    esac
done

# Validate required arguments
if [[ -z "$CONTAINER_APP_NAME" ]] || [[ -z "$RESOURCE_GROUP" ]] || [[ -z "$SUBSCRIPTION_ID" ]]; then
    print_error "Missing required arguments"
    usage
fi

# Banner
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  Azure Container App Permissions Verification"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Verify Azure CLI is installed
if ! command -v az &> /dev/null; then
    print_error "Azure CLI is not installed"
    exit 1
fi

# Set the subscription context
az account set --subscription "$SUBSCRIPTION_ID" 2>/dev/null

# Get the managed identity principal ID
print_info "Retrieving managed identity..."
PRINCIPAL_ID=$(az containerapp show \
    --name "$CONTAINER_APP_NAME" \
    --resource-group "$RESOURCE_GROUP" \
    --subscription "$SUBSCRIPTION_ID" \
    --query "identity.principalId" -o tsv 2>/dev/null || echo "")

if [[ -z "$PRINCIPAL_ID" ]] || [[ "$PRINCIPAL_ID" == "None" ]]; then
    print_error "Container App does not have a managed identity"
    exit 1
fi
print_success "Managed Identity Principal ID: $PRINCIPAL_ID"

# Get service principal details
CONTAINER_APP_SP=$(az ad sp show --id "$PRINCIPAL_ID" --query "{id:id,appId:appId}" -o json 2>/dev/null || echo "")
CONTAINER_APP_SP_ID=$(echo "$CONTAINER_APP_SP" | jq -r '.id')
CONTAINER_APP_APP_ID=$(echo "$CONTAINER_APP_SP" | jq -r '.appId')

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Azure RBAC Roles"
echo "───────────────────────────────────────────────────────────────"
echo ""

SUBSCRIPTION_SCOPE="/subscriptions/$SUBSCRIPTION_ID"

# Check Azure RBAC roles
REQUIRED_RBAC_ROLES=(
    "User Access Administrator"
    "Reader"
    "Key Vault Secrets User"
    "Key Vault Reader"
    "Key Vault Crypto User"
)

MISSING_RBAC=0
for ROLE in "${REQUIRED_RBAC_ROLES[@]}"; do
    ASSIGNMENT=$(az role assignment list \
        --assignee "$PRINCIPAL_ID" \
        --role "$ROLE" \
        --scope "$SUBSCRIPTION_SCOPE" \
        --query "[0].id" -o tsv 2>/dev/null || echo "")
    
    if [[ -n "$ASSIGNMENT" ]]; then
        print_success "$ROLE"
    else
        print_error "$ROLE (MISSING)"
        MISSING_RBAC=$((MISSING_RBAC + 1))
    fi
done

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Microsoft Graph API Permissions"
echo "───────────────────────────────────────────────────────────────"
echo ""

# Microsoft Graph API Application ID
GRAPH_API_ID="00000003-0000-0000-c000-000000000000"

# Required permissions (using parallel arrays for bash 3.2 compatibility)
GRAPH_PERMISSION_NAMES=("User.Read.All" "Group.Read.All" "Directory.Read.All")
GRAPH_PERMISSION_IDS=("df021288-bdef-4463-88db-98f22de89214" "5b567255-7703-4780-807c-7be8301ae99b" "7ab1d382-f21e-4acd-a863-ba3e13f7da61")

# Get Graph Service Principal ID
GRAPH_SP_ID=$(az ad sp show --id "$GRAPH_API_ID" --query "id" -o tsv 2>/dev/null || echo "")

# Get assigned permissions
ASSIGNED_PERMISSIONS=$(az rest \
    --method GET \
    --url "https://graph.microsoft.com/v1.0/servicePrincipals/$CONTAINER_APP_SP_ID/appRoleAssignments" \
    --query "value[?resourceId=='$GRAPH_SP_ID']" -o json 2>/dev/null || echo "[]")

MISSING_GRAPH=0
for i in "${!GRAPH_PERMISSION_NAMES[@]}"; do
    PERMISSION_NAME="${GRAPH_PERMISSION_NAMES[$i]}"
    PERMISSION_ID="${GRAPH_PERMISSION_IDS[$i]}"
    
    # Check if permission is assigned
    IS_ASSIGNED=$(echo "$ASSIGNED_PERMISSIONS" | jq -r ".[] | select(.appRoleId==\"$PERMISSION_ID\") | .id" 2>/dev/null || echo "")
    
    if [[ -n "$IS_ASSIGNED" ]]; then
        print_success "$PERMISSION_NAME (assigned)"
    else
        print_error "$PERMISSION_NAME (NOT ASSIGNED)"
        MISSING_GRAPH=$((MISSING_GRAPH + 1))
    fi
done

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Admin Consent Status"
echo "───────────────────────────────────────────────────────────────"
echo ""

# Check if admin consent has been granted by looking at the consent status
print_info "Checking admin consent status..."

# Get the app's oauth2PermissionGrants (delegated permissions) and appRoleAssignments (application permissions)
APP_PERMISSIONS=$(az ad app permission list --id "$CONTAINER_APP_APP_ID" -o json 2>/dev/null || echo "[]")

# For application permissions, we need to check if they have admin consent
CONSENT_STATUS=$(echo "$ASSIGNED_PERMISSIONS" | jq -r 'length')

if [[ "$CONSENT_STATUS" -gt 0 ]] && [[ $MISSING_GRAPH -eq 0 ]]; then
    print_success "Admin consent appears to be granted (all permissions assigned)"
else
    print_warning "Admin consent may not be granted or some permissions are missing"
    echo ""
    print_info "To grant admin consent, run:"
    echo "  az ad app permission admin-consent --id $CONTAINER_APP_APP_ID"
    echo ""
    print_info "Or visit:"
    echo "  https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationMenuBlade/~/CallAnAPI/appId/$CONTAINER_APP_APP_ID"
fi

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Summary"
echo "───────────────────────────────────────────────────────────────"
echo ""

TOTAL_ISSUES=$((MISSING_RBAC + MISSING_GRAPH))

if [[ $TOTAL_ISSUES -eq 0 ]]; then
    print_success "All permissions are correctly assigned!"
    echo ""
    print_info "If you're still seeing errors:"
    echo "  1. Wait 5-10 minutes for permission propagation"
    echo "  2. Restart the Container App to pick up new permissions"
    echo ""
    print_info "To restart the Container App:"
    echo "  REVISION=\$(az containerapp revision list \\"
    echo "    --name $CONTAINER_APP_NAME \\"
    echo "    --resource-group $RESOURCE_GROUP \\"
    echo "    --subscription $SUBSCRIPTION_ID \\"
    echo "    --query '[?properties.active==\`true\`].name' -o tsv)"
    echo ""
    echo "  az containerapp revision restart \\"
    echo "    --name $CONTAINER_APP_NAME \\"
    echo "    --resource-group $RESOURCE_GROUP \\"
    echo "    --subscription $SUBSCRIPTION_ID \\"
    echo "    --revision \$REVISION"
else
    print_error "Found $TOTAL_ISSUES issue(s):"
    if [[ $MISSING_RBAC -gt 0 ]]; then
        echo "  • $MISSING_RBAC missing Azure RBAC role(s)"
    fi
    if [[ $MISSING_GRAPH -gt 0 ]]; then
        echo "  • $MISSING_GRAPH missing Microsoft Graph permission(s)"
    fi
    echo ""
    print_info "Run the assignment script to fix:"
    echo "  ./azure-assign-container-app-permissions.sh \\"
    echo "    --container-app $CONTAINER_APP_NAME \\"
    echo "    --resource-group $RESOURCE_GROUP \\"
    echo "    --subscription-id $SUBSCRIPTION_ID"
fi

echo ""
