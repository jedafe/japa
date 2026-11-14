package atproto

// DID document construction (W3C DID Core + atproto method
// requirements). A member's doc must carry:
//   - verificationMethod: the repo signing key as publicKeyMultibase,
//     id "{did}#atproto", type "Multikey" (the modern form — Bluesky's
//     appview accepts it; the older EcdsaSecp256k1VerificationKey2019
//     publicKeyHex form is deprecated)
//   - service: #atproto_pds → type AtprotoPersonalDataServer with the
//     PDS public endpoint
//   - alsoKnownAs: the at:// handle URI
//
// Ref: https://atproto.com/specs/did + Section 127

// didDocument builds the did:web document for an account.
func didDocument(did, handle, pubMultibase, pdsURL string) map[string]any {
	return map[string]any{
		"@context": []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/multikey/v1",
			"https://w3id.org/security/suites/secp256k1-2019/v1",
		},
		"id":          did,
		"alsoKnownAs": []string{"at://" + handle},
		"verificationMethod": []map[string]any{
			{
				"id":                 did + "#atproto",
				"type":               "Multikey",
				"controller":         did,
				"publicKeyMultibase": pubMultibase,
			},
		},
		"service": []map[string]any{
			{
				"id":              "#atproto_pds",
				"type":            "AtprotoPersonalDataServer",
				"serviceEndpoint": pdsURL,
			},
		},
	}
}
