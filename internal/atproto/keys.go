package atproto

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/multiformats/go-multibase"
)

// K-256 (secp256k1) custody. The private key is a 32-byte scalar —
// stored only as internal/crypto ciphertext (same pattern as
// federated_actors.private_key_enc); the public half is published in the
// DID doc as a multibase-encoded multicodec key.
//
// multicodec secp256k1-pub = 0xE7 → uvarint [0xE7 0x01], prefixed to the
// 33-byte compressed public key, then multibase base58btc ("z…").
// This is the encoding Bluesky's appview expects in
// verificationMethod.publicKeyMultibase — byte-exactness matters.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md

const secp256k1PubMulticodec = 0xE7

// generateSigningKey creates a K-256 keypair. Returns the private scalar
// hex-encoded (for ciphertext storage) and the publicKeyMultibase string
// for the DID document.
func generateSigningKey() (privHex, pubMultibase string, err error) {
	priv, err := secp.GeneratePrivateKey()
	if err != nil {
		return "", "", fmt.Errorf("atproto: keygen: %w", err)
	}
	pub, err := publicKeyMultibase(priv.PubKey())
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(priv.Serialize()), pub, nil
}

// publicKeyMultibase encodes a secp256k1 public key as
// multibase(base58btc, uvarint(secp256k1-pub) || compressed33).
func publicKeyMultibase(pub *secp.PublicKey) (string, error) {
	prefix := make([]byte, 0, binary.MaxVarintLen64)
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], secp256k1PubMulticodec)
	prefix = append(prefix, buf[:n]...)
	encoded, err := multibase.Encode(multibase.Base58BTC, append(prefix, pub.SerializeCompressed()...))
	if err != nil {
		return "", fmt.Errorf("atproto: multibase encode: %w", err)
	}
	return encoded, nil
}

// parsePrivateKeyHex restores a stored scalar for signing (A2 commits).
func parsePrivateKeyHex(privHex string) (*secp.PrivateKey, error) {
	raw, err := hex.DecodeString(privHex)
	if err != nil {
		return nil, fmt.Errorf("atproto: bad key material: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("atproto: bad scalar length %d", len(raw))
	}
	return secp.PrivKeyFromBytes(raw), nil
}
