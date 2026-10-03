# Gateway contract build profiles

The default fixture compares the operations and fields consumed by the CLI with a Gateway checkout. Its source revision and spec digests record provenance; compatible additions can pass even when the producer has advanced.

For packaging that requires an exact producer contract, create a separate manifest from an immutable producer revision. The optional output path and HTTPS producer URL leave the default public fixture unchanged:

```sh
ruby scripts/update-gateway-contract.rb "$GATEWAY_REPO" "$GATEWAY_REVISION" "$PROFILE" "$PRODUCER_URL"
GATEWAY_REPO="$GATEWAY_REPO" GATEWAY_CONTRACT_MANIFEST="$PROFILE" GATEWAY_CONTRACT_EXACT=1 go test ./pkg/api -run TestGatewayContract -count=1
make build GATEWAY_CONTRACT_MANIFEST="$PROFILE" VERSION="$CANDIDATE_VERSION"
```

Use an absolute profile path when running tests: Go runs package tests from the package directory. The selected profile must identify a trusted producer. Exact mode rejects a different checkout HEAD or either spec digest before checking consumed operations and fields. A dirty checkout with changed spec bytes also fails.

The build stamps the selected manifest's `swagger_sha256` into `loxicmd version -o json`. Keep the manifest, both spec digests, source revision and binary checksums together in packaging evidence; the binary identity field alone does not attest the complete profile. Give a changed local binary a distinct candidate version. An external profile and local build do not create an official release or a signed build attestation.
