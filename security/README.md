# security

Reusable authentication primitives.

- `jwt`: user access-token verification (EdDSA/JWKS; explicit legacy HS256 migration fallback).
- `jwks`: concurrency-safe Ed25519 JWKS cache and PEM helpers.
- `s2s`: service identity, EdDSA service-token verifier, issuer-side signer, and outbound HTTP token transport.
- `echo`: Echo middleware for user JWT and S2S auth.
- `grpc`: gRPC S2S server/client interceptors.

Ordinary services should verify service tokens using public keys. Private signing keys belong only to explicitly authorized token issuers.
