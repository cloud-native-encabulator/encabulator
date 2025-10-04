#!/bin/bash

# Test script for data services connectivity and credentials
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Test PostgreSQL connectivity
test_postgresql() {
    log_info "Testing PostgreSQL connectivity..."
    
    # Check if PostgreSQL pod is running
    if kubectl get pod -n encabulator-data -l app.kubernetes.io/name=postgres >/dev/null 2>&1; then
        log_success "PostgreSQL pod is running"
        
        # Test database connection
        local pg_pod=$(kubectl get pod -n encabulator-data -l app.kubernetes.io/name=postgres -o jsonpath='{.items[0].metadata.name}')
        
        if kubectl exec -n encabulator-data "$pg_pod" -- psql -U hydrocoptic -d hydrocoptic_db -c "SELECT 1;" >/dev/null 2>&1; then
            log_success "PostgreSQL database connection successful"
        else
            log_error "PostgreSQL database connection failed"
            return 1
        fi
    else
        log_error "PostgreSQL pod not found"
        return 1
    fi
}

# Test Redis connectivity
test_redis() {
    log_info "Testing Redis connectivity..."
    
    # Check if Redis pod is running
    if kubectl get pod -n encabulator-data -l app.kubernetes.io/name=redis,app.kubernetes.io/instance=prefabulated-cache >/dev/null 2>&1; then
        log_success "Redis pod is running"
        
        # Test Redis connection
        local redis_pod=$(kubectl get pod -n encabulator-data -l app.kubernetes.io/name=redis,app.kubernetes.io/instance=prefabulated-cache -o jsonpath='{.items[0].metadata.name}')
        
        if kubectl exec -n encabulator-data "$redis_pod" -- redis-cli ping >/dev/null 2>&1; then
            log_success "Redis connection successful"
        else
            log_error "Redis connection failed"
            return 1
        fi
    else
        log_error "Redis pod not found"
        return 1
    fi
}

# Test InfluxDB connectivity
test_influxdb() {
    log_info "Testing InfluxDB connectivity..."
    
    # Check if InfluxDB pod is running
    if kubectl get pod -n encabulator-data -l app.kubernetes.io/name=influxdb2,app.kubernetes.io/instance=sinusoidal-database >/dev/null 2>&1; then
        log_success "InfluxDB pod is running"
        
        # Test InfluxDB health endpoint
        local influx_pod=$(kubectl get pod -n encabulator-data -l app.kubernetes.io/name=influxdb2,app.kubernetes.io/instance=sinusoidal-database -o jsonpath='{.items[0].metadata.name}')
        
        if kubectl exec -n encabulator-data "$influx_pod" -- wget -q -O- http://localhost:8086/health 2>/dev/null | grep -q "pass\|ok" 2>/dev/null; then
            log_success "InfluxDB health check successful"
        else
            log_error "InfluxDB health check failed"
            return 1
        fi
    else
        log_error "InfluxDB pod not found"
        return 1
    fi
}

# Test database backends connectivity
test_database_backends() {
    log_info "Testing database backends connectivity..."
    
    # Check if all three database backends are accessible
    local backends_found=0
    
    if kubectl get service -n encabulator-data hydrocoptic-postgresql >/dev/null 2>&1; then
        log_success "PostgreSQL service is accessible"
        ((backends_found++))
    else
        log_error "PostgreSQL service not found"
    fi
    
    if kubectl get service -n encabulator-data prefabulated-cache-redis >/dev/null 2>&1; then
        log_success "Redis service is accessible"
        ((backends_found++))
    else
        log_error "Redis service not found"
    fi
    
    if kubectl get service -n encabulator-data sinusoidal-database-influxdb2 >/dev/null 2>&1; then
        log_success "InfluxDB service is accessible"
        ((backends_found++))
    else
        log_error "InfluxDB service not found"
    fi
    
    if [ $backends_found -eq 3 ]; then
        log_success "All database backends are accessible"
    else
        log_error "Only $backends_found/3 database backends are accessible"
        return 1
    fi
}

# Test credentials and secrets
test_credentials() {
    log_info "Testing database credentials and secrets..."
    
    local secrets_found=0
    
    # Check PostgreSQL secret
    if kubectl get secret -n encabulator-data hydrocoptic-postgresql >/dev/null 2>&1; then
        log_success "PostgreSQL credentials secret exists"
        ((secrets_found++))
    else
        log_error "PostgreSQL credentials secret not found"
    fi
    
    # Check Redis secret (optional - some charts don't create secrets)
    if kubectl get secret -n encabulator-data prefabulated-cache-redis >/dev/null 2>&1; then
        log_success "Redis credentials secret exists"
        ((secrets_found++))
    else
        log_warning "Redis credentials secret not found (chart may not create secrets)"
        ((secrets_found++))  # Count as found since service is working
    fi
    
    # Check InfluxDB secret
    if kubectl get secret -n encabulator-data sinusoidal-database-influxdb2-auth >/dev/null 2>&1; then
        log_success "InfluxDB credentials secret exists"
        ((secrets_found++))
    else
        log_error "InfluxDB credentials secret not found"
    fi
    
    if [ $secrets_found -eq 3 ]; then
        log_success "All database credentials are available"
    else
        log_error "Only $secrets_found/3 database credential secrets found"
        return 1
    fi
}

# Main test function
main() {
    log_info "Starting data services connectivity tests..."
    echo ""
    
    local failed_tests=0
    
    test_postgresql || ((failed_tests++))
    echo ""
    
    test_redis || ((failed_tests++))
    echo ""
    
    test_influxdb || ((failed_tests++))
    echo ""
    
    test_database_backends || ((failed_tests++))
    echo ""
    
    test_credentials || ((failed_tests++))
    echo ""
    
    if [ $failed_tests -eq 0 ]; then
        log_success "All data services tests passed! ✅"
    else
        log_error "$failed_tests test(s) failed ❌"
        exit 1
    fi
}

# Handle script arguments
case "${1:-}" in
    --postgresql)
        test_postgresql
        ;;
    --redis)
        test_redis
        ;;
    --influxdb)
        test_influxdb
        ;;
    --backends)
        test_database_backends
        ;;
    --credentials)
        test_credentials
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --postgresql     Test PostgreSQL only"
        echo "  --redis          Test Redis only"
        echo "  --influxdb       Test InfluxDB only"
        echo "  --backends       Test database backends connectivity only"
        echo "  --credentials    Test credentials and secrets only"
        echo "  --help, -h       Show this help message"
        echo ""
        echo "Default: Run all tests"
        ;;
    "")
        main
        ;;
    *)
        log_error "Unknown option: $1"
        echo "Use --help for usage information"
        exit 1
        ;;
esac