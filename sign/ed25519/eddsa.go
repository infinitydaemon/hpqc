// SPDX-FileCopyrightText: (c) 2023 David Stainton and Yawning Angel
// SPDX-License-Identifier: AGPL-3.0-only

// Package is our ed25519 wrapper type which also conforms to our generic interfaces for signature schemes.
package ed25519

import (
	"crypto/ed25519"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/blake2b"

	"filippo.io/edwards25519"

	"github.com/katzenpost/hpqc/nike/x25519"
	"github.com/katzenpost/hpqc/sign"
	"github.com/katzenpost/hpqc/rand"
	"github.com/katzenpost/hpqc/util"
)

const (
	// PublicKeySize is the size of a serialized PublicKey in bytes (32 bytes).
	PublicKeySize = ed25519.PublicKeySize

	// PrivateKeySize is the size of a serialized PrivateKey in bytes (64 bytes).
	PrivateKeySize = ed25519.PrivateKeySize

	// SignatureSize is the size of a serialized Signature in bytes (64 bytes).
	SignatureSize = ed25519.SignatureSize

	keyType = "ed25519"
)

var errInvalidKey = errors.New("eddsa: invalid key")

// Scheme implements our sign.Scheme interface using the ed25519 wrapper.
type scheme struct{}

var sch sign.Scheme = &scheme{}

// Scheme returns a sign Scheme interface.
func Scheme() sign.Scheme { return sch }

// NewEmptyPublicKey returns an empty sign.PublicKey
func (s *scheme) NewEmptyPublicKey() sign.PublicKey {
	return new(PublicKey)
}

func (s *scheme) NewKeypair() (sign.PrivateKey, sign.PublicKey) {
	privKey, err := NewKeypair(rand.Reader)
	if err != nil {
		panic(err)
	}

	return privKey, privKey.PublicKey()
}

func (s *scheme) UnmarshalBinaryPublicKey(b []byte) (sign.PublicKey, error) {
	pubKey := new(PublicKey)
	err := pubKey.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}

func (s *scheme) UnmarshalBinaryPrivateKey(b []byte) (sign.PrivateKey, error) {
	privKey := new(PrivateKey)
	err := privKey.FromBytes(b)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

func (s *scheme) SignatureSize() int {
	return SignatureSize
}

func (s *scheme) PublicKeySize() int {
	return PublicKeySize
}

func (s *scheme) PrivateKeySize() int {
	return PrivateKeySize
}

func (s *scheme) Name() string {
	return "ED25519"
}

type PrivateKey struct {
	pubKey  PublicKey
	privKey ed25519.PrivateKey
}

// InternalPtr returns a pointer to the internal (`golang.org/x/crypto/ed25519`)
// data structure.  Most people should not use this.
func (p *PrivateKey) InternalPtr() *ed25519.PrivateKey {
	return &p.privKey
}

func (p *PrivateKey) KeyType() string {
	return "ED25519 PRIVATE KEY"
}

func (p *PrivateKey) Sign(message []byte) (signature []byte) {
	return ed25519.Sign(p.privKey, message)
}

func (p *PrivateKey) Reset() {
	p.pubKey.Reset()
	util.ExplicitBzero(p.privKey)
}

func (p *PrivateKey) Bytes() []byte {
	return p.privKey
}

// FromBytes deserializes the byte slice b into the PrivateKey.
func (p *PrivateKey) FromBytes(b []byte) error {
	if len(b) != PrivateKeySize {
		return errInvalidKey
	}

	p.privKey = make([]byte, PrivateKeySize)
	copy(p.privKey, b)
	p.pubKey.pubKey = p.privKey.Public().(ed25519.PublicKey)
	p.pubKey.rebuildB64String()
	return nil
}

// Identity returns the key's identity, in this case it's our
// public key in bytes.
func (p *PrivateKey) Identity() []byte {
	return p.PublicKey().Bytes()
}

// PublicKey returns the PublicKey corresponding to the PrivateKey.
func (p *PrivateKey) PublicKey() *PublicKey {
	return &p.pubKey
}

// PublicKey is the EdDSA public key using ed25519.
type PublicKey struct {
	pubKey    ed25519.PublicKey
	b64String string
}

// ToECDH converts the PublicKey to the corresponding ecdh.PublicKey.
func (p *PublicKey) ToECDH() *x25519.PublicKey {
	ed_pub, _ := new(edwards25519.Point).SetBytes(p.Bytes())
	r := new(x25519.PublicKey)
	if r.FromBytes(ed_pub.BytesMontgomery()) != nil {
		panic("edwards.Point from pub.BytesMontgomery failed, impossible. ")
	}
	return r
}

// InternalPtr returns a pointer to the internal (`golang.org/x/crypto/ed25519`)
// data structure.  Most people should not use this.
func (k *PublicKey) InternalPtr() *ed25519.PublicKey {
	return &k.pubKey
}

func (p *PublicKey) KeyType() string {
	return "ED25519 PUBLIC KEY"
}

func (p *PublicKey) Sum256() [32]byte {
	return blake2b.Sum256(p.Bytes())
}

func (p *PublicKey) Equal(pubKey sign.PublicKey) bool {
	return hmac.Equal(p.pubKey[:], pubKey.(*PublicKey).pubKey[:])
}

func (p *PublicKey) Verify(signature, message []byte) bool {
	return ed25519.Verify(p.pubKey, message, signature)
}

func (p *PublicKey) Reset() {
	util.ExplicitBzero(p.pubKey)
	p.b64String = "[scrubbed]"
}

func (p *PublicKey) Bytes() []byte {
	return p.pubKey
}

// ByteArray returns the raw public key as an array suitable for use as a map
// key.
func (p *PublicKey) ByteArray() [PublicKeySize]byte {
	var pk [PublicKeySize]byte
	copy(pk[:], p.pubKey[:])
	return pk
}

func (p *PublicKey) rebuildB64String() {
	p.b64String = base64.StdEncoding.EncodeToString(p.Bytes())
}

func (p *PublicKey) FromBytes(data []byte) error {
	if len(data) != PublicKeySize {
		return errInvalidKey
	}

	p.pubKey = make([]byte, PublicKeySize)
	copy(p.pubKey, data)
	p.rebuildB64String()
	return nil
}

func (p *PublicKey) MarshalBinary() ([]byte, error) {
	return p.Bytes(), nil
}

func (p *PublicKey) UnmarshalBinary(data []byte) error {
	return p.FromBytes(data)
}

// NewKeypair generates a new PrivateKey sampled from the provided entropy
// source.
func NewKeypair(r io.Reader) (*PrivateKey, error) {
	pubKey, privKey, err := ed25519.GenerateKey(r)
	if err != nil {
		return nil, err
	}

	k := new(PrivateKey)
	k.privKey = privKey
	k.pubKey.pubKey = pubKey
	k.pubKey.rebuildB64String()
	return k, nil
}
