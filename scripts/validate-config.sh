#!/usr/bin/env bash
#
# validate-config.sh - Validates ani-model-service configuration
#
# This script validates all required environment variables and configuration
# for the ani-model-service before deployment or startup. It checks:
# - Required environment variables
# - MinIO vs Storage gRPC configuration
# - TLS certificate files
# - Kubernetes permissions (when running in cluster)
#
# Exit codes:
#   0 - All validations passed
#   1 - One or more validations failed

set -euo pipefail

# Color codes for output
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    RESET='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    BOLD=''
    RESET=''
fi

# Track validation state
VALIDATION_FAILED=0
MISSING_VARS=()
WARNINGS=()

# Helper functions
print_header() {
    echo -e "${BOLD}${BLUE}=== $1 ===${RESET}"
}

print_success() {
    echo -e "${GREEN}✓${RESET} $1"
}

print_error() {
    echo -e "${RED}✗${RESET} $1"
    VALIDATION_FAILED=1
}

print_warning() {
    echo -e "${YELLOW}⚠${RESET} $1"
    WARNINGS+=("$1")
}

print_info() {
    echo -e "${BLUE}ℹ${RESET} $1"
}

check_required_var() {
    local var_name="$1"
    local var_value="${!var_name:-}"

    if [[ -z "${var_value// }" ]]; then
        MISSING_VARS+=("$var_name")
        print_error "Missing required variable: $var_name"
        return 1
    else
        print_success "$var_name is set"
        return 0
    fi
}

check_optional_var() {
    local var_name="$1"
    local var_value="${!var_name:-}"

    if [[ -z "${var_value// }" ]]; then
        print_info "$var_name is not set (optional)"
        return 0
    else
        print_success "$var_name is set"
        return 0
    fi
}

check_file_exists() {
    local file_path="$1"
    local description="$2"

    if [[ ! -f "$file_path" ]]; then
        print_error "$description not found: $file_path"
    else
        print_success "$description exists: $file_path"
    fi
}

check_file_readable() {
    local file_path="$1"
    local description="$2"

    if [[ ! -r "$file_path" ]]; then
        print_error "$description not readable: $file_path"
    else
        print_success "$description is readable: $file_path"
    fi
}

validate_pem_certificate() {
    local cert_path="$1"
    local description="$2"

    if ! openssl x509 -in "$cert_path" -noout -text &>/dev/null; then
        print_error "$description is not a valid X.509 certificate: $cert_path"
    else
        print_success "$description is a valid X.509 certificate"

        # Show certificate details
        local subject
        subject=$(openssl x509 -in "$cert_path" -noout -subject 2>/dev/null | sed 's/subject=//')
        local expiry
        expiry=$(openssl x509 -in "$cert_path" -noout -enddate 2>/dev/null | sed 's/notAfter=//')

        print_info "  Subject: $subject"
        print_info "  Expires: $expiry"

        # Check if certificate is expired
        if ! openssl x509 -in "$cert_path" -noout -checkend 0 &>/dev/null; then
            print_warning "Certificate has expired: $cert_path"
        fi

        # Warn if expiring within 30 days
        if ! openssl x509 -in "$cert_path" -noout -checkend 2592000 &>/dev/null; then
            print_warning "Certificate expires within 30 days: $cert_path"
        fi
    fi
}

# Main validation logic
main() {
    echo -e "${BOLD}ani-model-service Configuration Validator${RESET}"
    echo ""

    # 1. Core Database Configuration
    print_header "Database Configuration"
    check_required_var "ANI_DATABASE_DSN"
    echo ""

    # 2. Storage Backend Configuration
    print_header "Storage Backend Configuration"

    HAS_MINIO=false
    HAS_STORAGE_GRPC=false

    if [[ -n "${ANI_MINIO_ENDPOINT:-}" && "${ANI_MINIO_ENDPOINT// }" != "" ]]; then
        HAS_MINIO=true
        print_info "MinIO storage backend detected"
    fi

    if [[ -n "${ANI_STORAGE_GRPC_ADDR:-}" && "${ANI_STORAGE_GRPC_ADDR// }" != "" ]]; then
        HAS_STORAGE_GRPC=true
        print_info "Storage gRPC backend detected"
    fi

    # Validate exactly one storage backend is configured
    if [[ "$HAS_MINIO" == false && "$HAS_STORAGE_GRPC" == false ]]; then
        print_error "No storage backend configured. Set either ANI_MINIO_ENDPOINT or ANI_STORAGE_GRPC_ADDR"
        MISSING_VARS+=("ANI_MINIO_ENDPOINT or ANI_STORAGE_GRPC_ADDR")
    elif [[ "$HAS_MINIO" == true && "$HAS_STORAGE_GRPC" == true ]]; then
        print_error "Multiple storage backends configured. Set only one of ANI_MINIO_ENDPOINT or ANI_STORAGE_GRPC_ADDR"
    else
        print_success "Exactly one storage backend is configured"
    fi
    echo ""

    # 3. MinIO-specific Configuration
    if [[ "$HAS_MINIO" == true ]]; then
        print_header "MinIO Configuration"
        check_required_var "ANI_MINIO_ENDPOINT"
        check_required_var "ANI_MINIO_ACCESS_KEY"
        check_required_var "ANI_MINIO_SECRET_KEY"

        # Optional MinIO settings
        check_optional_var "ANI_MINIO_SECURE"
        check_optional_var "ANI_MINIO_BUCKET"
        check_optional_var "ANI_MINIO_TENANT_BUCKETS"

        # MinIO import worker requirements
        print_info "Checking MinIO import worker configuration..."
        check_required_var "ANI_IMPORT_JOB_IMAGE"
        check_required_var "ANI_IMPORT_MINIO_SECRET"
        check_required_var "ANI_IMPORT_KUBERNETES_NAMESPACE"

        # Optional import worker settings
        check_optional_var "ANI_IMPORT_WORKER_OWNER"
        check_optional_var "ANI_IMPORT_PROVIDER_SECRET"
        check_optional_var "ANI_IMPORT_STORAGE_CLASS"
        echo ""
    fi

    # 4. Storage gRPC Configuration
    if [[ "$HAS_STORAGE_GRPC" == true ]]; then
        print_header "Storage gRPC Configuration"
        check_required_var "ANI_STORAGE_GRPC_ADDR"
        check_required_var "ANI_STORAGE_GRPC_SERVER_NAME"

        # TLS Certificate Configuration
        print_info "Checking TLS certificate configuration..."

        CA_CERT_PATH="${ANI_STORAGE_GRPC_CA_CERT:-}"
        CA_CERT_PEM="${ANI_STORAGE_GRPC_CA_CERT_PEM:-}"

        if [[ -n "$CA_CERT_PATH" && -n "$CA_CERT_PEM" ]]; then
            print_warning "Both ANI_STORAGE_GRPC_CA_CERT and ANI_STORAGE_GRPC_CA_CERT_PEM are set. File path takes precedence."
        fi

        if [[ -n "$CA_CERT_PATH" ]]; then
            print_info "Using CA certificate from file: $CA_CERT_PATH"
            check_file_exists "$CA_CERT_PATH" "CA certificate file"
            check_file_readable "$CA_CERT_PATH" "CA certificate file"
            validate_pem_certificate "$CA_CERT_PATH" "CA certificate"
        elif [[ -n "$CA_CERT_PEM" ]]; then
            print_info "Using CA certificate from environment variable ANI_STORAGE_GRPC_CA_CERT_PEM"

            # Validate PEM format
            if echo "$CA_CERT_PEM" | openssl x509 -noout -text &>/dev/null; then
                print_success "CA certificate PEM is valid"
            else
                print_error "CA certificate PEM in ANI_STORAGE_GRPC_CA_CERT_PEM is invalid"
            fi
        else
            print_info "Using system certificate pool (default)"
        fi
        echo ""
    fi

    # 5. Inference Service Configuration (Optional)
    print_header "Inference Service Configuration"
    check_optional_var "ANI_INFERENCE_GRPC_ADDR"
    check_optional_var "ANI_INFERENCE_GRPC_SERVER_NAME"
    echo ""

    # 6. External Provider Configuration (Optional)
    print_header "External Provider Configuration"
    check_optional_var "ANI_HUGGINGFACE_BASE_URL"
    check_optional_var "ANI_MODELSCOPE_BASE_URL"
    echo ""

    # 7. HTTP Client Configuration (Optional)
    print_header "HTTP Client Configuration"
    check_optional_var "ANI_IMPORTER_HTTP_TIMEOUT"
    check_optional_var "ANI_IMPORTER_DIAL_TIMEOUT"
    echo ""

    # 8. Kubernetes Permissions Check (if in cluster)
    print_header "Kubernetes Permissions"

    if [[ -f "/var/run/secrets/kubernetes.io/serviceaccount/token" ]]; then
        print_info "Running in Kubernetes cluster, checking permissions..."

        # Check if kubectl is available
        if command -v kubectl &>/dev/null; then
            NAMESPACE="${ANI_IMPORT_KUBERNETES_NAMESPACE:-ani-model}"

            # Check if we can create jobs
            if kubectl auth can-i create jobs -n "$NAMESPACE" &>/dev/null; then
                print_success "Has permission to create Jobs in namespace $NAMESPACE"
            else
                print_error "Missing permission to create Jobs in namespace $NAMESPACE"
            fi

            # Check if we can get jobs
            if kubectl auth can-i get jobs -n "$NAMESPACE" &>/dev/null; then
                print_success "Has permission to get Jobs in namespace $NAMESPACE"
            else
                print_error "Missing permission to get Jobs in namespace $NAMESPACE"
            fi

            # Check if we can watch jobs
            if kubectl auth can-i watch jobs -n "$NAMESPACE" &>/dev/null; then
                print_success "Has permission to watch Jobs in namespace $NAMESPACE"
            else
                print_error "Missing permission to watch Jobs in namespace $NAMESPACE"
            fi

            # Check if we can delete jobs
            if kubectl auth can-i delete jobs -n "$NAMESPACE" &>/dev/null; then
                print_success "Has permission to delete Jobs in namespace $NAMESPACE"
            else
                print_warning "Missing permission to delete Jobs in namespace $NAMESPACE (may cause cleanup issues)"
            fi
        else
            print_warning "kubectl not found, skipping Kubernetes permissions check"
        fi
    else
        print_info "Not running in Kubernetes cluster, skipping permissions check"
    fi
    echo ""

    # Summary
    print_header "Validation Summary"

    if [[ ${#MISSING_VARS[@]} -gt 0 ]]; then
        echo -e "${RED}${BOLD}Validation Failed${RESET}"
        echo ""
        echo -e "${RED}Missing required variables:${RESET}"
        for var in "${MISSING_VARS[@]}"; do
            echo -e "  ${RED}•${RESET} $var"
        done
        echo ""
    fi

    if [[ ${#WARNINGS[@]} -gt 0 ]]; then
        echo -e "${YELLOW}Warnings (${#WARNINGS[@]}):${RESET}"
        for warning in "${WARNINGS[@]}"; do
            echo -e "  ${YELLOW}•${RESET} ${warning#⚠ }"
        done
        echo ""
    fi

    if [[ $VALIDATION_FAILED -eq 0 ]]; then
        echo -e "${GREEN}${BOLD}✓ All validations passed${RESET}"
        if [[ ${#WARNINGS[@]} -gt 0 ]]; then
            echo -e "${YELLOW}  Note: ${#WARNINGS[@]} warning(s) reported above${RESET}"
        fi
        return 0
    else
        echo -e "${RED}${BOLD}✗ Validation failed with errors${RESET}"
        echo ""
        echo "Please fix the errors above and run this script again."
        return 1
    fi
}

# Run main function
main "$@"
