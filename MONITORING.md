# EvalHub Monitoring System

## Overview

The EvalHub monitoring system provides comprehensive observability, health checking, and metrics collection for the application. It's designed to be production-ready with environment-specific configurations.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Dashboard     │    │   Handlers      │    │   Router        │
│   (Core Logic)  │◄───┤   (HTTP Layer)  │◄───┤   (Routes)      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Metrics        │    │  Security       │    │  Utilities      │
│  Collector      │    │  Monitoring     │    │  & Helpers      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Components

### 1. Dashboard Core (`internal/monitoring/dashboard.go`)
- **Purpose**: Core business logic for health checks and metrics
- **Key Methods**:
  - `GetSystemHealth()` - Comprehensive health assessment
  - `GetComprehensiveMetrics()` - Application metrics
  - `GetDashboardData()` - Combined health and metrics data

### 2. Handler Files
- **Health Handlers** (`internal/handlers/web/health_handlers.go`)
- **Metrics Handlers** (`internal/handlers/web/metrics_handlers.go`)
- **Dashboard Handlers** (`internal/handlers/web/dashboard_handlers.go`)
- **Security Handlers** (`internal/handlers/web/security_handlers.go`)

### 3. Router Integration (`router/router.go`)
- **Purpose**: Route setup and monitoring integration
- **Key Functions**:
  - `SetupMonitoringRoutes()` - Sets up all monitoring endpoints
  - `SetupErrorMonitoring()` - Configures error tracking

## API Endpoints

### Public Endpoints
```
GET /health                    # Basic health check
GET /status                    # Application status
GET /healthz                   # Kubernetes liveness probe
GET /readyz                    # Kubernetes readiness probe
```

### Internal Monitoring Endpoints
```
GET /internal/health           # Detailed health check
GET /internal/metrics          # Comprehensive metrics
GET /internal/dashboard        # Full dashboard data
```

### Specialized Monitoring
```
GET /internal/metrics/api            # API-specific metrics
GET /internal/metrics/performance    # Performance metrics
GET /internal/metrics/database      # Database metrics
GET /internal/metrics/security      # Security metrics
GET /internal/metrics/prometheus    # Prometheus format
```

### Dashboard Views
```
GET /internal/dashboard/monitoring    # Monitoring-focused view
GET /internal/dashboard/alerts      # Alerts and issues
GET /internal/dashboard/performance  # Performance dashboard
GET /internal/dashboard/system      # System information
GET /internal/dashboard/quick       # Quick stats
```

### Security Monitoring
```
POST /api/security/csp-report        # CSP violation reports
POST /api/security/violations        # Security violations
POST /api/security/hsts-report       # HSTS violations
GET  /internal/security/health       # Security health
GET  /internal/security/config       # Security configuration
```

## Environment Configuration

### Development
- All endpoints accessible
- Detailed error information
- Relaxed security policies
- Verbose logging

### Staging  
- Internal endpoints require authentication
- Moderate security policies
- Debug logging enabled
- Stack traces in errors

### Production
- Strict access controls for internal endpoints
- Maximum security policies
- Error details hidden
- Performance optimized

## Integration Guide

### 1. Initialize Dashboard
```go
// Create metrics collector
metricsCollector := middleware.NewMetricsCollector(logger)

// Initialize dashboard
dashboard := monitoring.NewDashboard(
    metricsCollector, 
    logger, 
    "1.0.0",           // version
    "production",      // environment
)
```

### 2. Setup Routes
```go
// Get base router
mux := router.SetupRouter().(*http.ServeMux)

// Add monitoring routes
router.SetupMonitoringRoutes(mux, dashboard, logger)

// Add error monitoring
errorTracker := middleware.NewErrorTracker(logger)
router.SetupErrorMonitoring(mux, errorTracker)
```

### 3. Configure Middleware
```go
// Add metrics middleware to chain
handler = middleware.MetricsMiddleware(metricsCollector)(handler)

// Add other middleware...
handler = errorHandlingStack(handler)
handler = recoveryStack(handler)
handler = securityStack(handler)
```

## Data Structures

### System Health Response
```json
{
  "status": "healthy|degraded|unhealthy",
  "timestamp": "2024-01-01T00:00:00Z",
  "uptime": "1h30m45s",
  "version": "1.0.0",
  "environment": "production",
  "components": {
    "database": {
      "status": "healthy",
      "response_time": "5ms",
      "details": { ... }
    },
    "api": { ... },
    "cache": { ... }
  },
  "performance": {
    "requests_per_second": 125.5,
    "average_latency": "50ms",
    "error_rate": 0.1,
    "availability": 99.9
  },
  "resources": {
    "memory": {
      "value": "512MB",
      "usage": 65.2,
      "status": "healthy"
    }
  },
  "summary": {
    "performance_score": 95.5,
    "reliability_score": 98.2,
    "operational_score": 92.1
  }
}
```

## Security

### Access Control
- **Development**: No restrictions
- **Production**: IP whitelisting, auth tokens, user agent validation

### Authorization Methods
1. **Header-based**: `X-Internal-Auth: <token>`
2. **IP Whitelist**: Configured allowed IPs
3. **User Agent**: Internal service identification

### Security Headers
- Content Security Policy (CSP)
- HTTP Strict Transport Security (HSTS)
- X-Frame-Options
- X-Content-Type-Options

## Monitoring Features

### Health Scoring
- **Performance Score**: Based on latency, error rate, availability
- **Reliability Score**: Component health, alerts
- **Operational Score**: Resource usage, critical issues

### Alert Types
- **Performance**: High latency, error rates
- **Resource**: Memory, database connections
- **Component**: Service health issues
- **Security**: Violations, suspicious activity

### Background Monitoring
- Periodic health checks (30s intervals)
- Metrics logging (5min intervals)
- Automatic alerting on critical issues

## Usage Examples

### Basic Health Check
```bash
curl http://localhost:8080/health
```

### Detailed Monitoring (Internal)
```bash
curl -H "X-Internal-Auth: your-token" \
     http://localhost:8080/internal/dashboard
```

### Prometheus Metrics
```bash
curl -H "X-Internal-Auth: your-token" \
     http://localhost:8080/internal/metrics/prometheus
```

## Configuration

### Environment Variables
```bash
# Basic configuration
GO_ENV=production
APP_VERSION=1.0.0
INTERNAL_AUTH_TOKEN=your-secure-token

# Security configuration  
CORS_ALLOWED_ORIGINS=https://yourdomain.com,https://api.yourdomain.com
CSP_SCRIPT_SRC='self'
HSTS_MAX_AGE=31536000

# Monitoring configuration
METRICS_ENABLED=true
HEALTH_CHECK_INTERVAL=30s
BACKGROUND_MONITORING=true
```

### Production Checklist
- [ ] Set `INTERNAL_AUTH_TOKEN` for internal endpoints
- [ ] Configure `CORS_ALLOWED_ORIGINS` for your domains
- [ ] Set up IP whitelisting for monitoring services
- [ ] Configure alerting systems to monitor `/health`
- [ ] Set up log aggregation for monitoring data
- [ ] Test all monitoring endpoints
- [ ] Verify security headers are applied

## Troubleshooting

### Common Issues

1. **403 Forbidden on Internal Endpoints**
   - Check `INTERNAL_AUTH_TOKEN` is set
   - Verify IP is in allowed list
   - Confirm user agent is recognized

2. **Metrics Not Updating**
   - Ensure `MetricsMiddleware` is in middleware chain
   - Check metrics collector initialization
   - Verify background monitoring is running

3. **Health Checks Failing**
   - Check database connectivity
   - Verify all components are initialized
   - Review component health check implementations

### Debug Mode
```bash
# Enable debug logging
GO_ENV=development

# Check specific component health
curl http://localhost:8080/internal/health
```

## Contributing

When adding new monitoring features:

1. **Core Logic**: Add to `dashboard.go`
2. **HTTP Layer**: Create handler in appropriate handler file
3. **Routes**: Add to `SetupMonitoringRoutes()`
4. **Documentation**: Update this README
5. **Tests**: Add comprehensive tests for new functionality

## Performance Considerations

- Health checks have 10s timeout
- Background monitoring runs every 30s
- Metrics collection is lightweight
- Internal endpoints use caching where appropriate
- Production mode optimizes for performance over detail