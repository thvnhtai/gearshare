# Local SAML test IdP

GearShare's SAML SP (`internal/auth/saml.go`) needs an IdP's metadata URL
to actually authenticate against — this repo does not ship a live IdP,
per the project's ["explicitly thin-but-real
treatments"](../../../README.md#explicitly-thin-but-real-treatments).

To exercise the SAML flow locally, either:

1. **`samltest.id`** — a free, public test IdP. Register the SP metadata
   from `GET /saml/metadata` there, then set
   `SAML_IDP_METADATA_URL=https://samltest.id/saml/idp` in `.env`.
2. **Keycloak**, running locally with a SAML client configured — export
   its realm's IdP metadata URL and point `SAML_IDP_METADATA_URL` at it.
3. **`crewjam/saml`'s own `samlidp` package** — the same library GearShare's
   SP uses also ships a minimal IdP implementation
   (`github.com/crewjam/saml/samlidp`), suitable for a fully offline,
   fully local SP↔IdP round trip without any external dependency. Not
   wired into this repo's `cmd/` today, but it's the natural next step if
   you want to exercise SAML without a network dependency at all.

Once `SAML_IDP_METADATA_URL` is set, `cmd/api/main.go` builds a real
`samlsp.Middleware` at boot (self-signed SP cert generated automatically —
see `auth.GenerateSelfSignedCert`) and serves `/saml/metadata` and
`/saml/acs` for real.
