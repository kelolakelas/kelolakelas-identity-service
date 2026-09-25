# KEL-66: Password reset and gateway session revocation

Status: proposed for delivery with KEL-66.

## Decision

Identity stores only the SHA-256 digest of a 256-bit random reset token in PostgreSQL, with expiry and consumption timestamp. Issuance serializes on the user row, replaces previous outstanding tokens, and sends a link through the existing Resend integration. Consumption locks the user then the token in a transaction, updates the password hash and user-specific `session_valid_after`, marks the token used and removes other tokens atomically. An unregistered address, an email delivery error, and a successful delivery produce the same public request response. Delivery errors are logged without the token or email address.

Identity's internal session-check route verifies the signed JWT itself and reads the user's durable boundary for each request; it is not exposed through gateway public routes. Gateway checks every protected request after local signature verification and before proxying. A revoked/unknown session gets 401; a failed identity call or database read gets 503 (fail closed), without caching a positive result. Existing claims and downstream authorization checks remain unchanged. This adds an identity round trip per protected gateway request and makes identity/PostgreSQL availability necessary for these requests.

JWT `iat` has second resolution. A reset sets the boundary at the first whole second after confirmation and post-reset login issues a JWT at or after that boundary. If another reset occurs before that second arrives, its boundary advances strictly beyond the previous one so even an intervening login is revoked. Pre-reset JWTs therefore compare strictly before the current boundary, including ones issued within the same wall-clock second; no token refresh is attempted. Gateway does not directly inspect the database. The new token is valid immediately because the current JWT parser does not reject a future `iat`; expiration is calculated from the issuance time. Clock skew and changing this JWT parser's future-iat policy require coordinated review.

## Alternatives and limits

A local gateway denylist/cache cannot guarantee immediate revocation on multiple replicas or during a cache outage; per-user durable state is used instead. Neither academic nor billing directly checks this boundary when bypassing gateway; do not describe direct service access as revoked. The `/internal/session/check` identity route is intentionally not registered on gateway; deployment must keep direct identity access private to trusted services. Roll out identity migration and identity service before gateway; old gateway remains stateless until upgraded. Rolling back gateway restores stateless behavior, so rollback during a security incident must be explicitly evaluated.
