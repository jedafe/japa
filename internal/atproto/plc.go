// did-method-plc operations (§127 A6-2). A plc_operation is the signed
// identity record the PLC directory logs — genesis ops derive the DID
// from their own hash, updates chain off the audit-log head CID.
//
// Wire shape (matches plc.directory /log/audit entries):
//
//	{"type":"plc_operation",
//	 "rotationKeys":["did:key:z…"],              rotation pub, not the repo key
//	 "verificationMethods":{"atproto":"did:key:z…"},  repo signing pub
//	 "alsoKnownAs":["at://{handle}"],
//	 "services":{"atproto_pds":{"type":"AtprotoPersonalDataServer",
//	                           "endpoint":PDSURL}},
//	 "prev":null|"bafy…",                        CID string in BOTH forms
//	 "sig":"base64url(raw R||S)"}                rotation-key signature
//
// sig covers dag-cbor of the op MINUS sig. The DID is
// did:plc: + base32lower(sha256(dagcbor(signed_op)))[:24] — for genesis
// only; update ops don't re-derive.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md
// Ref: https://github.com/did-method-plc/did-method-plc
package atproto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	atcrypto "github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	basicnode "github.com/ipld/go-ipld-prime/node/basicnode"
	jcrypto "github.com/jedafe/japa/internal/crypto"
)

const (
	plcOpType            = "plc_operation"
	plcPDSServiceType    = "AtprotoPersonalDataServer"
	plcSigningMethodName = "atproto"
	plcPDSServiceName    = "atproto_pds"
	plcDIDPrefix         = "did:plc:"
	// plcDIDSuffixLen is the base32 hash truncation the method fixes.
	plcDIDSuffixLen = 24
)

// ErrPLCRejected is permanent: the directory refused the operation —
// retrying the identical op can't succeed (malformed op, bad sig).
var ErrPLCRejected = errors.New("atproto: plc operation rejected")

// plcOperation is one signed identity operation. prev is nil on
// genesis; sig is filled by signPLCOp. rotationDIDKeys is a list — the
// method supports multiple rotation keys (a future rotation-rotation
// adds a second) even though we mint exactly one.
type plcOperation struct {
	rotationDIDKeys []string // did:key of the rotation key(s) (op signer)
	signingDIDKey   string   // did:key of the repo signing key
	alsoKnownAs     []string // ["at://{handle}"]
	pdsEndpoint     string
	prev            *cid.Cid
	sig             string
}

// plcHTTPClient bounds directory calls — submit/resolve/audit. Kept
// off the shared transport like crawlHTTPClient (sync.go).
var plcHTTPClient = &http.Client{Timeout: 10 * time.Second}

// newPLCOperation assembles the unsigned op. rotationPubMB and
// signingPubMB are the publicKeyMultibase encodings keys.go emits.
func newPLCOperation(rotationPubMB, signingPubMB, handle, pdsEndpoint string, prev *cid.Cid) *plcOperation {
	return &plcOperation{
		rotationDIDKeys: []string{"did:key:" + rotationPubMB},
		signingDIDKey:   "did:key:" + signingPubMB,
		alsoKnownAs:     []string{"at://" + handle},
		pdsEndpoint:     pdsEndpoint,
		prev:            prev,
	}
}

// node renders the op as an IPLD map — withSig toggles the sig field so
// one builder serves both the signing input (minus sig) and the hashed
// genesis object (with sig). prev encodes as an IPLD link; null on
// genesis.
func (op *plcOperation) node(withSig bool) (ipld.Node, error) {
	return qp.BuildMap(basicnode.Prototype.Map, -1, func(ma ipld.MapAssembler) {
		qp.MapEntry(ma, "type", qp.String(plcOpType))
		qp.MapEntry(ma, "rotationKeys", qp.List(int64(len(op.rotationDIDKeys)), func(la ipld.ListAssembler) {
			for _, rk := range op.rotationDIDKeys {
				qp.ListEntry(la, qp.String(rk))
			}
		}))
		qp.MapEntry(ma, "verificationMethods", qp.Map(int64(1), func(ma ipld.MapAssembler) {
			qp.MapEntry(ma, plcSigningMethodName, qp.String(op.signingDIDKey))
		}))
		qp.MapEntry(ma, "alsoKnownAs", qp.List(int64(len(op.alsoKnownAs)), func(la ipld.ListAssembler) {
			for _, aka := range op.alsoKnownAs {
				qp.ListEntry(la, qp.String(aka))
			}
		}))
		qp.MapEntry(ma, "services", qp.Map(int64(1), func(ma ipld.MapAssembler) {
			qp.MapEntry(ma, plcPDSServiceName, qp.Map(int64(2), func(ma ipld.MapAssembler) {
				qp.MapEntry(ma, "type", qp.String(plcPDSServiceType))
				qp.MapEntry(ma, "endpoint", qp.String(op.pdsEndpoint))
			}))
		}))
		// prev is the CID STRING in both encodings — the op log's cids
		// only match when prev serializes as text, not a tag-42 link
		// (verified against live directory entries).
		if op.prev == nil {
			qp.MapEntry(ma, "prev", qp.Null())
		} else {
			qp.MapEntry(ma, "prev", qp.String(op.prev.String()))
		}
		if withSig && op.sig != "" {
			qp.MapEntry(ma, "sig", qp.String(op.sig))
		}
	})
}

// encode serializes the op to dag-cbor (canonical: sorted keys,
// definite lengths) — the signing input when withSig is false, the
// DID-derivation preimage when true.
func (op *plcOperation) encode(withSig bool) ([]byte, error) {
	n, err := op.node(withSig)
	if err != nil {
		return nil, fmt.Errorf("atproto: plc op node: %w", err)
	}
	var buf bytes.Buffer
	if err := dagcbor.Encode(n, &buf); err != nil {
		return nil, fmt.Errorf("atproto: plc op dag-cbor: %w", err)
	}
	return buf.Bytes(), nil
}

// signPLCOp signs dag-cbor(op minus sig) with the ROTATION key and sets
// op.sig = base64url(raw R||S, no padding) — the same low-S 64-byte
// form atcrypto emits for commit signatures.
func signPLCOp(op *plcOperation, rotationPrivHex string) error {
	payload, err := op.encode(false)
	if err != nil {
		return err
	}
	priv, err := parsePrivateKeyHex(rotationPrivHex)
	if err != nil {
		return err
	}
	key, err := atcrypto.ParsePrivateBytesK256(priv.Serialize())
	if err != nil {
		return fmt.Errorf("atproto: rotation k256 key: %w", err)
	}
	sig, err := key.HashAndSign(payload)
	if err != nil {
		return fmt.Errorf("atproto: plc op sign: %w", err)
	}
	op.sig = base64.RawURLEncoding.EncodeToString(sig)
	return nil
}

// did derives the genesis DID: did:plc: + base32lower(sha256(dagcbor(
// signed_op)))[:24]. Only valid once sig is set.
func (op *plcOperation) did() (string, error) {
	signed, err := op.encode(true)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(signed)
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return plcDIDPrefix + strings.ToLower(suffix)[:plcDIDSuffixLen], nil
}

// json renders the wire form — prev becomes a plain CID string (or
// null), the rest is literal. Key order is irrelevant on the wire.
func (op *plcOperation) json() ([]byte, error) {
	var prev any
	if op.prev != nil {
		prev = op.prev.String()
	}
	return json.Marshal(map[string]any{
		"type":                plcOpType,
		"rotationKeys":        op.rotationDIDKeys,
		"verificationMethods": map[string]string{plcSigningMethodName: op.signingDIDKey},
		"alsoKnownAs":         op.alsoKnownAs,
		"services": map[string]any{
			plcPDSServiceName: map[string]string{
				"type":     plcPDSServiceType,
				"endpoint": op.pdsEndpoint,
			},
		},
		"prev": prev,
		"sig":  op.sig,
	})
}

// submitPLCOp POSTs the signed op to {plcURL}/{did}. 2xx is logged
// success; 4xx is permanent (ErrPLCRejected); 5xx/network errors are
// transient and retryable by the caller.
func (s *Service) submitPLCOp(ctx context.Context, did string, op *plcOperation) error {
	body, err := op.json()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(s.cfg.plcURL(), "/")+"/"+did, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("atproto: plc submit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := plcHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("atproto: plc submit: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: status %d: %s", ErrPLCRejected, resp.StatusCode, strings.TrimSpace(string(msg)))
	default:
		return fmt.Errorf("atproto: plc submit status %d", resp.StatusCode)
	}
}

// plcDIDRegistered resolves {plcURL}/{did} — 200 means the DID exists
// (a genesis DID can only exist if OUR op created it: the hash covers
// our keys); 404 means not yet submitted. Other statuses error.
func (s *Service) plcDIDRegistered(ctx context.Context, did string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(s.cfg.plcURL(), "/")+"/"+did, nil)
	if err != nil {
		return false, fmt.Errorf("atproto: plc resolve request: %w", err)
	}
	resp, err := plcHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("atproto: plc resolve: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusOK:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("atproto: plc resolve status %d", resp.StatusCode)
	}
}

// plcAuditHead returns the CID of the DID's latest non-nullified op —
// the `prev` an update op chains onto. The audit log is authoritative;
// no local head-tracking state can drift from it.
func (s *Service) plcAuditHead(ctx context.Context, did string) (cid.Cid, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(s.cfg.plcURL(), "/")+"/"+did+"/log/audit", nil)
	if err != nil {
		return cid.Undef, fmt.Errorf("atproto: plc audit request: %w", err)
	}
	resp, err := plcHTTPClient.Do(req)
	if err != nil {
		return cid.Undef, fmt.Errorf("atproto: plc audit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return cid.Undef, fmt.Errorf("atproto: plc audit status %d", resp.StatusCode)
	}
	var entries []struct {
		CID       string `json:"cid"`
		Nullified bool   `json:"nullified"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&entries); err != nil {
		return cid.Undef, fmt.Errorf("atproto: plc audit decode: %w", err)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Nullified {
			continue
		}
		c, err := cid.Decode(entries[i].CID)
		if err != nil {
			return cid.Undef, fmt.Errorf("atproto: plc audit cid: %w", err)
		}
		return c, nil
	}
	return cid.Undef, fmt.Errorf("atproto: plc audit empty for %s", did)
}

// plcURL resolves the directory base (JC_ATPROTO_PLC_URL).
func (c Config) plcURL() string {
	if c.PLCURL != "" {
		return strings.TrimRight(c.PLCURL, "/")
	}
	return "https://plc.directory"
}

// rotationKeyMaterial decrypts the stored rotation-key ciphertext and
// returns the private scalar hex (for signing) plus the public
// publicKeyMultibase (for rotationKeys in the op).
func rotationKeyMaterial(rotationKeyEnc string) (privHex, pubMultibase string, err error) {
	privHex, err = jcrypto.Decrypt(rotationKeyEnc)
	if err != nil {
		return "", "", fmt.Errorf("atproto: rotation key decrypt: %w", err)
	}
	priv, err := parsePrivateKeyHex(privHex)
	if err != nil {
		return "", "", err
	}
	pub, err := publicKeyMultibase(priv.PubKey())
	if err != nil {
		return "", "", err
	}
	return privHex, pub, nil
}
