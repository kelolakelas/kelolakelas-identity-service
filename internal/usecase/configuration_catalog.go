package usecase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

var configurationKeysByApplication = map[string]string{
	"web":         "GATEWAY_API_URL NEXT_PUBLIC_APP_URL AUTH_COOKIE_NAME TENANT_ID_COOKIE_NAME NODE_ENV",
	"api-gateway": "JWT_SECRET APP_URL PORT IDENTITY_SERVICE_URL ACADEMIC_SERVICE_URL BILLING_SERVICE_URL REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB RATE_LIMIT_REQUESTS RATE_LIMIT_WINDOW_SECONDS RATE_LIMIT_PUBLIC_REQUESTS RATE_LIMIT_PROTECTED_REQUESTS RATE_LIMIT_LOGIN_REQUESTS RATE_LIMIT_REGISTER_REQUESTS RATE_LIMIT_WEBHOOK_REQUESTS RATE_LIMIT_WEBHOOK_WINDOW_SECONDS PROXY_UPSTREAM_TIMEOUT_SECONDS PROXY_MAX_BODY_BYTES SERVER_READ_HEADER_TIMEOUT_SECONDS SERVER_READ_TIMEOUT_SECONDS SERVER_WRITE_TIMEOUT_SECONDS SERVER_IDLE_TIMEOUT_SECONDS TRUSTED_PROXY_CIDRS TRUSTED_CLIENT_IP_HEADER",
	"identity":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB JWT_SECRET PORT APP_URL RESEND_API_KEY RESEND_FROM_EMAIL GOOGLE_MAPS_API_KEY GOOGLE_MAPS_GEOCODING_ENABLED GOOGLE_MAPS_TIMEOUT_SECONDS PERMISSION_REQUIRE_TENANT_ID",
	"academic":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME IDENTITY_GRPC_HOST BILLING_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL PORT JWT_SECRET CATALOG_TENANT_INFO_TTL_MINUTES CATALOG_TENANT_INFO_TIMEOUT_MS",
	"billing":     "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME PORT DUITKU_API_BASE_URL DUITKU_API_KEY DUITKU_MERCHANT_CODE DUITKU_CALLBACK_URL DUITKU_RETURN_URL ACADEMIC_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL JWT_SECRET SUBSCRIPTION_WORKER_ENABLED SUBSCRIPTION_WORKER_INTERVAL_MINUTES SUBSCRIPTION_PAYMENT_REMINDER_INTERVAL_DAYS SUBSCRIPTION_PAYMENT_EXPIRY_PERIOD_DAYS PAYMENT_RECONCILIATION_WORKER_ENABLED PAYMENT_RECONCILIATION_WORKER_INTERVAL_MINUTES PAYMENT_RECONCILIATION_MAX_ATTEMPTS TRANSACTION_EXPIRY_WORKER_ENABLED TRANSACTION_EXPIRY_WORKER_INTERVAL_MINUTES TRANSACTION_CLAIM_TIMEOUT_MINUTES RESEND_API_KEY RESEND_FROM_EMAIL IDENTITY_GRPC_HOST IDENTITY_PERMISSION_TIMEOUT_MS",
}

var configurationTypes = map[string]string{
	"GATEWAY_API_URL": "url", "NEXT_PUBLIC_APP_URL": "url", "APP_URL": "url",
	"IDENTITY_SERVICE_URL": "url", "ACADEMIC_SERVICE_URL": "url", "BILLING_SERVICE_URL": "url",
	"DUITKU_API_BASE_URL": "url", "DUITKU_CALLBACK_URL": "url", "DUITKU_RETURN_URL": "url",
	"REDIS_TLS": "boolean", "GOOGLE_MAPS_GEOCODING_ENABLED": "boolean",
	"PERMISSION_REQUIRE_TENANT_ID": "boolean", "SUBSCRIPTION_WORKER_ENABLED": "boolean",
	"PAYMENT_RECONCILIATION_WORKER_ENABLED": "boolean", "TRANSACTION_EXPIRY_WORKER_ENABLED": "boolean",
	"DB_PORT": "integer", "PORT": "integer", "REDIS_PORT": "integer", "REDIS_DB": "integer",
	"RATE_LIMIT_REQUESTS": "integer", "RATE_LIMIT_WINDOW_SECONDS": "integer", "RATE_LIMIT_PUBLIC_REQUESTS": "integer",
	"RATE_LIMIT_PROTECTED_REQUESTS": "integer", "RATE_LIMIT_LOGIN_REQUESTS": "integer",
	"RATE_LIMIT_REGISTER_REQUESTS": "integer", "RATE_LIMIT_WEBHOOK_REQUESTS": "integer",
	"RATE_LIMIT_WEBHOOK_WINDOW_SECONDS": "integer", "PROXY_UPSTREAM_TIMEOUT_SECONDS": "integer",
	"PROXY_MAX_BODY_BYTES": "integer", "SERVER_READ_HEADER_TIMEOUT_SECONDS": "integer",
	"SERVER_READ_TIMEOUT_SECONDS": "integer", "SERVER_WRITE_TIMEOUT_SECONDS": "integer",
	"SERVER_IDLE_TIMEOUT_SECONDS": "integer", "GOOGLE_MAPS_TIMEOUT_SECONDS": "integer",
	"CATALOG_TENANT_INFO_TTL_MINUTES": "integer", "CATALOG_TENANT_INFO_TIMEOUT_MS": "integer",
	"SUBSCRIPTION_WORKER_INTERVAL_MINUTES": "integer", "SUBSCRIPTION_PAYMENT_REMINDER_INTERVAL_DAYS": "integer",
	"SUBSCRIPTION_PAYMENT_EXPIRY_PERIOD_DAYS": "integer", "PAYMENT_RECONCILIATION_WORKER_INTERVAL_MINUTES": "integer",
	"PAYMENT_RECONCILIATION_MAX_ATTEMPTS": "integer", "TRANSACTION_EXPIRY_WORKER_INTERVAL_MINUTES": "integer",
	"TRANSACTION_CLAIM_TIMEOUT_MINUTES": "integer", "IDENTITY_PERMISSION_TIMEOUT_MS": "integer",
}

var configurationDefaults = map[string]string{
	"GATEWAY_API_URL": "required", "NEXT_PUBLIC_APP_URL": "http://localhost:3000", "AUTH_COOKIE_NAME": "auth_token",
	"TENANT_ID_COOKIE_NAME": "tenant_id", "NODE_ENV": "runtime default",
	"APP_URL": "http://localhost:3000", "PORT": "service-specific default",
	"IDENTITY_SERVICE_URL": "http://localhost:8080", "ACADEMIC_SERVICE_URL": "http://localhost:8081",
	"BILLING_SERVICE_URL": "http://localhost:8082", "DATABASE_URL": "unset",
	"DB_HOST": "localhost", "DB_PORT": "5432", "DB_SSLMODE": "disable", "DB_CHANNEL_BINDING": "disable",
	"DB_USER": "postgres", "DB_PASSWORD": "postgres", "DB_NAME": "service-specific default",
	"REDIS_HOST": "localhost", "REDIS_PORT": "6379", "REDIS_USERNAME": "default", "REDIS_PASSWORD": "empty",
	"REDIS_TLS": "false", "REDIS_DB": "0", "JWT_SECRET": "required",
	"RATE_LIMIT_REQUESTS": "60", "RATE_LIMIT_WINDOW_SECONDS": "60", "RATE_LIMIT_PUBLIC_REQUESTS": "60",
	"RATE_LIMIT_PROTECTED_REQUESTS": "120", "RATE_LIMIT_LOGIN_REQUESTS": "5", "RATE_LIMIT_REGISTER_REQUESTS": "10",
	"RATE_LIMIT_WEBHOOK_REQUESTS": "120", "RATE_LIMIT_WEBHOOK_WINDOW_SECONDS": "60",
	"PROXY_UPSTREAM_TIMEOUT_SECONDS": "30", "PROXY_MAX_BODY_BYTES": "1048576",
	"SERVER_READ_HEADER_TIMEOUT_SECONDS": "5", "SERVER_READ_TIMEOUT_SECONDS": "30",
	"SERVER_WRITE_TIMEOUT_SECONDS": "60", "SERVER_IDLE_TIMEOUT_SECONDS": "120",
	"TRUSTED_PROXY_CIDRS": "empty", "TRUSTED_CLIENT_IP_HEADER": "empty",
	"RESEND_API_KEY": "unset", "RESEND_FROM_EMAIL": "unset", "GOOGLE_MAPS_API_KEY": "unset",
	"GOOGLE_MAPS_GEOCODING_ENABLED": "false", "GOOGLE_MAPS_TIMEOUT_SECONDS": "5",
	"PERMISSION_REQUIRE_TENANT_ID": "false", "IDENTITY_GRPC_HOST": "localhost:50051",
	"INTERNAL_SERVICE_CREDENTIAL": "required", "CATALOG_TENANT_INFO_TTL_MINUTES": "5",
	"CATALOG_TENANT_INFO_TIMEOUT_MS": "2000", "DUITKU_API_BASE_URL": "sandbox provider URL",
	"DUITKU_API_KEY": "required for invoice creation", "DUITKU_MERCHANT_CODE": "required for invoice creation",
	"DUITKU_CALLBACK_URL": "local service callback", "DUITKU_RETURN_URL": "callback URL",
	"SUBSCRIPTION_WORKER_ENABLED": "false", "SUBSCRIPTION_WORKER_INTERVAL_MINUTES": "1440",
	"SUBSCRIPTION_PAYMENT_REMINDER_INTERVAL_DAYS": "3", "SUBSCRIPTION_PAYMENT_EXPIRY_PERIOD_DAYS": "14",
	"PAYMENT_RECONCILIATION_WORKER_ENABLED": "true", "PAYMENT_RECONCILIATION_WORKER_INTERVAL_MINUTES": "1",
	"PAYMENT_RECONCILIATION_MAX_ATTEMPTS": "10", "TRANSACTION_EXPIRY_WORKER_ENABLED": "true",
	"TRANSACTION_EXPIRY_WORKER_INTERVAL_MINUTES": "5", "TRANSACTION_CLAIM_TIMEOUT_MINUTES": "10",
	"IDENTITY_PERMISSION_TIMEOUT_MS": "3000",
}

var sensitiveConfigurationKeys = map[string]bool{
	"DATABASE_URL": true, "DB_PASSWORD": true, "REDIS_PASSWORD": true, "JWT_SECRET": true,
	"RESEND_API_KEY": true, "GOOGLE_MAPS_API_KEY": true, "INTERNAL_SERVICE_CREDENTIAL": true,
	"DUITKU_API_KEY": true, "DUITKU_MERCHANT_CODE": true,
}

// Services load their environment at startup; web settings may be read at
// request time or embedded at build time. These describe the conservative
// deployment action needed, not an action performed by this control plane.
// There is no dynamic adapter.
var bootstrapConfigurationKeys = map[string]string{
	"web":         "GATEWAY_API_URL NEXT_PUBLIC_APP_URL AUTH_COOKIE_NAME TENANT_ID_COOKIE_NAME NODE_ENV",
	"api-gateway": "JWT_SECRET PORT IDENTITY_SERVICE_URL ACADEMIC_SERVICE_URL BILLING_SERVICE_URL REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB TRUSTED_PROXY_CIDRS TRUSTED_CLIENT_IP_HEADER",
	"identity":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB JWT_SECRET PORT RESEND_API_KEY GOOGLE_MAPS_API_KEY",
	"academic":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME IDENTITY_GRPC_HOST BILLING_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL PORT JWT_SECRET",
	"billing":     "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME PORT DUITKU_API_KEY DUITKU_MERCHANT_CODE ACADEMIC_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL JWT_SECRET RESEND_API_KEY IDENTITY_GRPC_HOST",
}

func catalogValidation(key, valueType string, sensitive bool) string {
	if sensitive {
		return "deployment-only: supply and validate in the secret manager; control plane rejects values"
	}
	const common = "control-plane: one non-null UTF-8 JSON scalar of at most 16384 bytes; "
	switch key {
	case "TRUSTED_CLIENT_IP_HEADER":
		return common + "valid HTTP header name or empty string; deployment validates proxy pairing"
	case "TRUSTED_PROXY_CIDRS":
		return common + "comma-separated IP/CIDR without zones or /0, or empty string; deployment validates proxy pairing"
	case "REDIS_DB":
		return common + "non-negative integer; deployment validates connectivity"
	}
	switch valueType {
	case "integer":
		return common + "positive integer; deployment may apply additional constraints"
	case "boolean":
		return common + "boolean; deployment may apply additional constraints"
	case "url":
		return common + "absolute HTTP(S) string without userinfo (optional URLs may be empty); deployment may apply additional constraints"
	default:
		return common + "non-empty string; deployment may apply additional constraints"
	}
}

var configurationDescriptions = map[string]string{
	"GATEWAY_API_URL":                         "Server-side API gateway origin used by web requests.",
	"NEXT_PUBLIC_APP_URL":                     "Public web origin used for canonical and Open Graph metadata.",
	"JWT_SECRET":                              "Shared HS256 signing and verification key; never managed as plaintext by this catalog.",
	"DATABASE_URL":                            "PostgreSQL connection URL; may embed database credentials and is catalog-only.",
	"INTERNAL_SERVICE_CREDENTIAL":             "Bearer credential for authenticated service-to-service calls; catalog-only.",
	"TRUSTED_PROXY_CIDRS":                     "Comma-separated proxy IPs or CIDRs trusted to provide the client address.",
	"TRUSTED_CLIENT_IP_HEADER":                "Header trusted proxies use to forward the original client address.",
	"PERMISSION_REQUIRE_TENANT_ID":            "Require tenant_id on internal identity CheckPermission calls.",
	"GOOGLE_MAPS_GEOCODING_ENABLED":           "Enable identity-service geocoding for tenant locations.",
	"SUBSCRIPTION_PAYMENT_EXPIRY_PERIOD_DAYS": "Duitku invoice validity and stored invoice expiry in days.",
	"TRANSACTION_CLAIM_TIMEOUT_MINUTES":       "Timeout before an abandoned invoice-creation claim can be taken over.",
}

func ConfigurationCatalog() []domain.ConfigurationDefinition {
	applications := make([]string, 0, len(configurationKeysByApplication))
	for application := range configurationKeysByApplication {
		applications = append(applications, application)
	}
	sort.Strings(applications)

	catalog := make([]domain.ConfigurationDefinition, 0, 120)
	for _, application := range applications {
		keys := strings.Fields(configurationKeysByApplication[application])
		sort.Strings(keys)
		for _, key := range keys {
			configurationType := configurationTypes[key]
			if configurationType == "" {
				configurationType = "string"
			}
			description := configurationDescriptions[key]
			if description == "" {
				description = fmt.Sprintf("Runtime setting consumed by the %s application.", application)
			}
			sensitive := sensitiveConfigurationKeys[key]
			bootstrapOnly := strings.Contains(" "+bootstrapConfigurationKeys[application]+" ", " "+key+" ")
			method := "restart"
			if sensitive {
				method = "rotation"
			} else if application == "web" && (key == "NEXT_PUBLIC_APP_URL" || key == "NODE_ENV") {
				method = "rebuild/redeploy"
			}
			defaultValue := configurationDefaults[key]
			if sensitive {
				// Even a development fallback can be a usable credential; never return it.
				defaultValue = ""
			}
			catalog = append(catalog, domain.ConfigurationDefinition{
				Application:       application,
				Owner:             application,
				Key:               key,
				Type:              configurationType,
				Required:          key == "JWT_SECRET" || key == "INTERNAL_SERVICE_CREDENTIAL" || key == "GATEWAY_API_URL",
				Default:           defaultValue,
				Description:       description,
				Sensitive:         sensitive,
				Validation:        catalogValidation(key, configurationType, sensitive),
				ApplicationMethod: method,
				BootstrapOnly:     bootstrapOnly,
			})
		}
	}
	return catalog
}

func findConfigurationDefinition(application, key string) (domain.ConfigurationDefinition, bool) {
	for _, definition := range ConfigurationCatalog() {
		if definition.Application == application && definition.Key == key {
			return definition, true
		}
	}
	return domain.ConfigurationDefinition{}, false
}
