#!/bin/bash

###############################################################################
# Azure Container App Permissions Assignment Script
#
# This script assigns all necessary Azure permissions to a Container App's
# managed identity to enable the agent to function properly.
#
# The agent requires the following permissions:
# 1. Azure RBAC: Read and manage role assignments and definitions
# 2. Microsoft Graph API: Read users and groups from Azure AD
# 3. Subscription Access: Read subscriptions and resource groups
#
# Usage:
#   ./azure-assign-container-app-permissions.sh \
#     --container-app <container-app-name> \
#     --resource-group <resource-group> \
#     --subscription-id <subscription-id> \
#     [--tenant-id <tenant-id>]
#
# Prerequisites:
#   - Azure CLI installed and configured (az login)
#   - Appropriate permissions to assign roles (Owner or User Access Administrator)
#   - Container App must have managed identity enabled
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
TENANT_ID=""
DRY_RUN=false

# Function to print colored messages
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to display usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Assigns necessary Azure permissions to a Container App's managed identity.

Required Options:
  --container-app NAME       Name of the Container App
  --resource-group NAME      Resource Group containing the Container App
  --subscription-id ID       Azure Subscription ID

Optional:
  --tenant-id ID            Azure Tenant ID (auto-detected if not provided)
  --dry-run                 Show what would be done without making changes
  -h, --help                Display this help message

Examples:
  # Basic usage
  $0 --container-app my-agent --resource-group my-rg --subscription-id 12345678-1234-1234-1234-123456789abc

  # With explicit tenant ID
  $0 --container-app my-agent --resource-group my-rg --subscription-id 12345678-1234-1234-1234-123456789abc --tenant-id 87654321-4321-4321-4321-cba987654321

  # Dry run to preview changes
  $0 --container-app my-agent --resource-group my-rg --subscription-id 12345678-1234-1234-1234-123456789abc --dry-run

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
        --tenant-id)
            TENANT_ID="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
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
echo "  Azure Container App Permissions Assignment Script"
echo "═══════════════════════════════════════════════════════════════"
echo ""

if [[ "$DRY_RUN" == true ]]; then
    print_warning "DRY RUN MODE - No changes will be made"
    echo ""
fi

# Verify Azure CLI is installed
if ! command -v az &> /dev/null; then
    print_error "Azure CLI is not installed. Please install it first."
    echo "Visit: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli"
    exit 1
fi

print_info "Checking Azure CLI authentication..."
if ! az account show &> /dev/null; then
    print_error "Not logged in to Azure. Please run 'az login' first."
    exit 1
fi

# Set the subscription context
print_info "Setting subscription context to: $SUBSCRIPTION_ID"
if [[ "$DRY_RUN" == false ]]; then
    az account set --subscription "$SUBSCRIPTION_ID"
fi
print_success "Subscription context set"

# Get tenant ID if not provided
if [[ -z "$TENANT_ID" ]]; then
    print_info "Retrieving tenant ID..."
    TENANT_ID=$(az account show --subscription "$SUBSCRIPTION_ID" --query tenantId -o tsv)
    print_success "Tenant ID: $TENANT_ID"
fi

# Check if Container App exists and has managed identity enabled
print_info "Verifying Container App: $CONTAINER_APP_NAME"
CONTAINER_APP_EXISTS=$(az containerapp show \
    --name "$CONTAINER_APP_NAME" \
    --resource-group "$RESOURCE_GROUP" \
    --subscription "$SUBSCRIPTION_ID" \
    --query "name" -o tsv 2>/dev/null || echo "")

if [[ -z "$CONTAINER_APP_EXISTS" ]]; then
    print_error "Container App '$CONTAINER_APP_NAME' not found in resource group '$RESOURCE_GROUP'"
    exit 1
fi
print_success "Container App found"

# Get the managed identity principal ID
print_info "Retrieving managed identity..."
PRINCIPAL_ID=$(az containerapp show \
    --name "$CONTAINER_APP_NAME" \
    --resource-group "$RESOURCE_GROUP" \
    --subscription "$SUBSCRIPTION_ID" \
    --query "identity.principalId" -o tsv 2>/dev/null || echo "")

if [[ -z "$PRINCIPAL_ID" ]] || [[ "$PRINCIPAL_ID" == "None" ]]; then
    print_error "Container App does not have a system-assigned managed identity enabled"
    print_info "Enable managed identity with:"
    echo "  az containerapp identity assign --name $CONTAINER_APP_NAME --resource-group $RESOURCE_GROUP --system-assigned"
    exit 1
fi
print_success "Managed Identity Principal ID: $PRINCIPAL_ID"

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Assigning Azure RBAC Permissions"
echo "───────────────────────────────────────────────────────────────"
echo ""

# Define the required Azure RBAC roles at subscription scope
# These permissions allow the agent to:
# 1. Read and create custom role definitions
# 2. Read and create role assignments
# 3. Read subscriptions and resource groups

print_info "The following Azure RBAC operations are required:"
echo "  • Microsoft.Authorization/roleAssignments/read"
echo "  • Microsoft.Authorization/roleAssignments/write"
echo "  • Microsoft.Authorization/roleDefinitions/read"
echo "  • Microsoft.Authorization/roleDefinitions/write"
echo "  • Microsoft.Resources/subscriptions/read"
echo "  • Microsoft.Resources/subscriptions/resourceGroups/read"
echo ""

# We'll assign the "User Access Administrator" role which provides the authorization permissions
# plus "Reader" role for subscription and resource group read access
# plus Key Vault roles for secrets and crypto operations
ROLES_TO_ASSIGN=(
    "User Access Administrator"  # Allows managing role assignments
    "Reader"                      # Allows reading subscription and resource group info
    "Key Vault Secrets User"     # Allows reading secrets from Key Vault
    "Key Vault Reader"            # Allows reading Key Vault metadata
    "Key Vault Crypto User"       # Allows cryptographic operations in Key Vault
)

SUBSCRIPTION_SCOPE="/subscriptions/$SUBSCRIPTION_ID"

for ROLE in "${ROLES_TO_ASSIGN[@]}"; do
    print_info "Assigning role: $ROLE"
    
    # Check if role assignment already exists
    EXISTING_ASSIGNMENT=$(az role assignment list \
        --assignee "$PRINCIPAL_ID" \
        --role "$ROLE" \
        --scope "$SUBSCRIPTION_SCOPE" \
        --query "[0].id" -o tsv 2>/dev/null || echo "")
    
    if [[ -n "$EXISTING_ASSIGNMENT" ]]; then
        print_warning "Role '$ROLE' is already assigned"
    else
        if [[ "$DRY_RUN" == true ]]; then
            print_info "[DRY RUN] Would assign role: $ROLE"
        else
            az role assignment create \
                --assignee "$PRINCIPAL_ID" \
                --role "$ROLE" \
                --scope "$SUBSCRIPTION_SCOPE" \
                --output none
            print_success "Assigned role: $ROLE"
        fi
    fi
done

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Assigning Microsoft Graph API Permissions"
echo "───────────────────────────────────────────────────────────────"
echo ""

print_info "The following Microsoft Graph API permissions are required:"
echo "  • User.Read.All (Application) - Read all users"
echo "  • Group.Read.All (Application) - Read all groups"
echo "  • Directory.Read.All (Application) - Read directory data"
echo ""

# Microsoft Graph API Application ID (constant across all Azure tenants)
GRAPH_API_ID="00000003-0000-0000-c000-000000000000"

# Required Microsoft Graph permissions (using parallel arrays for bash 3.2 compatibility)
GRAPH_PERMISSION_NAMES=("User.Read.All" "Group.Read.All" "Directory.Read.All")
GRAPH_PERMISSION_IDS=("df021288-bdef-4463-88db-98f22de89214" "5b567255-7703-4780-807c-7be8301ae99b" "7ab1d382-f21e-4acd-a863-ba3e13f7da61")

print_info "Retrieving Microsoft Graph Service Principal..."
GRAPH_SP_ID=$(az ad sp show --id "$GRAPH_API_ID" --query "id" -o tsv 2>/dev/null || echo "")

if [[ -z "$GRAPH_SP_ID" ]]; then
    print_error "Failed to retrieve Microsoft Graph Service Principal"
    exit 1
fi
print_success "Microsoft Graph Service Principal ID: $GRAPH_SP_ID"

# Get the managed identity's service principal object
print_info "Retrieving Container App Service Principal..."
CONTAINER_APP_SP=$(az ad sp show --id "$PRINCIPAL_ID" --query "{id:id,appId:appId}" -o json 2>/dev/null || echo "")

if [[ -z "$CONTAINER_APP_SP" ]]; then
    print_error "Failed to retrieve Container App Service Principal"
    print_warning "Managed identity may need time to propagate. Please wait a few minutes and try again."
    exit 1
fi

CONTAINER_APP_SP_ID=$(echo "$CONTAINER_APP_SP" | jq -r '.id')
CONTAINER_APP_APP_ID=$(echo "$CONTAINER_APP_SP" | jq -r '.appId')
print_success "Container App Service Principal ID: $CONTAINER_APP_SP_ID"

# Get existing app role assignments
print_info "Checking existing Microsoft Graph permissions..."
EXISTING_PERMISSIONS=$(az rest \
    --method GET \
    --url "https://graph.microsoft.com/v1.0/servicePrincipals/$CONTAINER_APP_SP_ID/appRoleAssignments" \
    --query "value[?resourceId=='$GRAPH_SP_ID'].appRoleId" -o tsv 2>/dev/null || echo "")

# Assign each required permission
for i in "${!GRAPH_PERMISSION_NAMES[@]}"; do
    PERMISSION_NAME="${GRAPH_PERMISSION_NAMES[$i]}"
    PERMISSION_ID="${GRAPH_PERMISSION_IDS[$i]}"
    
    print_info "Assigning permission: $PERMISSION_NAME"
    
    # Check if permission already exists
    if echo "$EXISTING_PERMISSIONS" | grep -q "$PERMISSION_ID"; then
        print_warning "Permission '$PERMISSION_NAME' is already assigned"
    else
        if [[ "$DRY_RUN" == true ]]; then
            print_info "[DRY RUN] Would assign permission: $PERMISSION_NAME"
        else
            # Assign the app role (permission)
            az rest \
                --method POST \
                --url "https://graph.microsoft.com/v1.0/servicePrincipals/$CONTAINER_APP_SP_ID/appRoleAssignments" \
                --headers "Content-Type=application/json" \
                --body "{
                    \"principalId\": \"$CONTAINER_APP_SP_ID\",
                    \"resourceId\": \"$GRAPH_SP_ID\",
                    \"appRoleId\": \"$PERMISSION_ID\"
                }" \
                --output none 2>/dev/null
            
            if [[ $? -eq 0 ]]; then
                print_success "Assigned permission: $PERMISSION_NAME"
            else
                print_error "Failed to assign permission: $PERMISSION_NAME"
                print_warning "You may need to grant admin consent manually via Azure Portal"
            fi
        fi
    fi
done

echo ""
echo "───────────────────────────────────────────────────────────────"
echo " Admin Consent Required"
echo "───────────────────────────────────────────────────────────────"
echo ""

print_warning "Microsoft Graph API permissions require admin consent"
print_info "To grant admin consent, visit:"
echo ""
echo "  https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationMenuBlade/~/CallAnAPI/appId/$CONTAINER_APP_APP_ID/isMSAApp~/false"
echo ""
print_info "Or run the following command:"
echo ""
echo "  az ad app permission admin-consent --id $CONTAINER_APP_APP_ID"
echo ""

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo " Summary"
echo "═══════════════════════════════════════════════════════════════"
echo ""

if [[ "$DRY_RUN" == true ]]; then
    print_info "DRY RUN completed - no changes were made"
else
    print_success "Permission assignment completed!"
fi

echo ""
echo "Container App:        $CONTAINER_APP_NAME"
echo "Resource Group:       $RESOURCE_GROUP"
echo "Subscription ID:      $SUBSCRIPTION_ID"
echo "Tenant ID:            $TENANT_ID"
echo "Principal ID:         $PRINCIPAL_ID"
echo ""

print_info "Azure RBAC Roles Assigned:"
for ROLE in "${ROLES_TO_ASSIGN[@]}"; do
    echo "  ✓ $ROLE"
done

echo ""
print_info "Microsoft Graph Permissions Assigned:"
for PERMISSION in "${!GRAPH_PERMISSIONS[@]}"; do
    echo "  ✓ $PERMISSION"
done

echo ""
print_warning "Next Steps:"
echo "  1. Grant admin consent for Microsoft Graph permissions (see URL above)"
echo "  2. Wait 5-10 minutes for permissions to propagate"
echo "  3. Restart the Container App to ensure it picks up the new permissions"
echo "  4. Test the agent's connectivity to Azure AD and RBAC"
echo ""

print_info "To restart the Container App, first get the active revision:"
echo "  REVISION=\$(az containerapp revision list \\"
echo "    --name $CONTAINER_APP_NAME \\"
echo "    --resource-group $RESOURCE_GROUP \\"
echo "    --subscription $SUBSCRIPTION_ID \\"
echo "    --query '[?properties.active==\`true\`].name' -o tsv)"
echo ""
echo "  Then restart it:"
echo "  az containerapp revision restart \\"
echo "    --name $CONTAINER_APP_NAME \\"
echo "    --resource-group $RESOURCE_GROUP \\"
echo "    --subscription $SUBSCRIPTION_ID \\"
echo "    --revision \$REVISION"
echo ""

print_success "Done!"
echo ""
