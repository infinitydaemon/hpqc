// Package hybrid defines several hybrid classical/quantum KEMs.
//
// KEMs are combined by hashing of shared secrets, cipher texts,
// public keys, etc, see
//
//	https://eprint.iacr.org/2018/024.pdf
//
// For deriving a KEM keypair deterministically and encapsulating
// deterministically, we expand a single seed to both using Blake2b hash and then XOF,
// so that a non-uniform seed (such as a shared secret generated by a hybrid
// KEM where one of the KEMs is weak) doesn't impact just one of the KEMs.

package hybrid

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/katzenpost/hpqc/kem"
	"github.com/katzenpost/hpqc/kem/pem"
	"github.com/katzenpost/hpqc/kem/util"
)

var (
	ErrUninitialized = errors.New("public or private key not initialized")
)

// Public key of a hybrid KEM.
type PublicKey struct {
	scheme *Scheme
	first  kem.PublicKey
	second kem.PublicKey
}

// Private key of a hybrid KEM.
type PrivateKey struct {
	scheme *Scheme
	first  kem.PrivateKey
	second kem.PrivateKey
}

// Scheme for a hybrid KEM.
type Scheme struct {
	name   string
	first  kem.Scheme
	second kem.Scheme
}

// New creates a new hybrid KEM given the first and second KEMs.
func New(name string, first kem.Scheme, second kem.Scheme) *Scheme {
	return &Scheme{
		name:   name,
		first:  first,
		second: second,
	}
}

func (sch *Scheme) Name() string { return sch.name }
func (sch *Scheme) PublicKeySize() int {
	return sch.first.PublicKeySize() + sch.second.PublicKeySize()
}

func (sch *Scheme) PrivateKeySize() int {
	return sch.first.PrivateKeySize() + sch.second.PrivateKeySize()
}

func (sch *Scheme) SeedSize() int {
	return sch.first.SeedSize() + sch.second.SeedSize()
}

func (sch *Scheme) SharedKeySize() int {
	return sch.first.SharedKeySize() + sch.second.SharedKeySize()
}

func (sch *Scheme) CiphertextSize() int {
	return sch.first.CiphertextSize() + sch.second.CiphertextSize()
}

func (sch *Scheme) EncapsulationSeedSize() int {
	return sch.first.EncapsulationSeedSize() + sch.second.EncapsulationSeedSize()
}

func (sk *PrivateKey) Scheme() kem.Scheme { return sk.scheme }
func (pk *PublicKey) Scheme() kem.Scheme  { return pk.scheme }

func (sk *PrivateKey) MarshalBinary() ([]byte, error) {
	if sk.first == nil || sk.second == nil {
		return nil, ErrUninitialized
	}
	first, err := sk.first.MarshalBinary()
	if err != nil {
		return nil, err
	}
	second, err := sk.second.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(first, second...), nil
}

func (sk *PublicKey) MarshalText() (text []byte, err error) {
	return pem.ToPublicPEMBytes(sk), nil
}

func (sk *PublicKey) UnmarshalText(text []byte) error {
	blob, err := pem.FromPublicPEMToBytes(text, sk.Scheme())
	if err != nil {
		return err
	}
	pubkey, err := sk.Scheme().UnmarshalBinaryPublicKey(blob)
	if err != nil {
		return err
	}
	var ok bool
	sk, ok = pubkey.(*PublicKey)
	if !ok {
		return errors.New("type assertion failed")
	}
	return nil
}

func (sk *PrivateKey) Equal(other kem.PrivateKey) bool {
	oth, ok := other.(*PrivateKey)
	if !ok {
		return false
	}
	return sk.first.Equal(oth.first) && sk.second.Equal(oth.second)
}

func (sk *PrivateKey) Public() kem.PublicKey {
	return &PublicKey{sk.scheme, sk.first.Public(), sk.second.Public()}
}

func (pk *PublicKey) Equal(other kem.PublicKey) bool {
	oth, ok := other.(*PublicKey)
	if !ok {
		return false
	}
	return pk.first.Equal(oth.first) && pk.second.Equal(oth.second)
}

func (pk *PublicKey) MarshalBinary() ([]byte, error) {
	if pk.first == nil || pk.second == nil {
		return nil, ErrUninitialized
	}
	first, err := pk.first.MarshalBinary()
	if err != nil {
		return nil, err
	}
	second, err := pk.second.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(first, second...), nil
}

func (sch *Scheme) GenerateKeyPair() (kem.PublicKey, kem.PrivateKey, error) {
	pk1, sk1, err := sch.first.GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}
	pk2, sk2, err := sch.second.GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}

	return &PublicKey{sch, pk1, pk2}, &PrivateKey{sch, sk1, sk2}, nil
}

func (sch *Scheme) DeriveKeyPair(seed []byte) (kem.PublicKey, kem.PrivateKey) {
	if len(seed) != sch.first.SeedSize()+sch.second.SeedSize() {
		panic(fmt.Sprintf("seed size must be %d", sch.first.SeedSize()+sch.second.SeedSize()))
	}

	pk1, sk1 := sch.first.DeriveKeyPair(seed[:sch.first.SeedSize()])
	pk2, sk2 := sch.second.DeriveKeyPair(seed[sch.first.SeedSize():])

	return &PublicKey{sch, pk1, pk2}, &PrivateKey{sch, sk1, sk2}
}

func (sch *Scheme) Encapsulate(pk kem.PublicKey) (ct, ss []byte, err error) {
	seed := make([]byte, sch.EncapsulationSeedSize())
	_, err = rand.Reader.Read(seed)
	if err != nil {
		return
	}
	return sch.EncapsulateDeterministically(pk, seed)
}

func (sch *Scheme) EncapsulateDeterministically(publicKey kem.PublicKey, seed []byte) (ct, ss []byte, err error) {
	if len(seed) != sch.EncapsulationSeedSize() {
		return nil, nil, kem.ErrSeedSize
	}

	first := seed[:sch.first.EncapsulationSeedSize()]
	second := seed[sch.first.EncapsulationSeedSize():]

	pub, ok := publicKey.(*PublicKey)
	if !ok {
		return nil, nil, kem.ErrTypeMismatch
	}

	ct1, ss1, err := sch.first.EncapsulateDeterministically(pub.first, first)
	if err != nil {
		return nil, nil, err
	}

	ct2, ss2, err := sch.second.EncapsulateDeterministically(pub.second, second)
	if err != nil {
		return nil, nil, err
	}

	ss = util.PairSplitPRF(ss1, ss2, ct1, ct2)

	return append(ct1, ct2...), ss, nil
}

func (sch *Scheme) Decapsulate(sk kem.PrivateKey, ct []byte) ([]byte, error) {
	if len(ct) != sch.CiphertextSize() {
		return nil, kem.ErrCiphertextSize
	}

	priv, ok := sk.(*PrivateKey)
	if !ok {
		return nil, kem.ErrTypeMismatch
	}

	firstSize := sch.first.CiphertextSize()
	ss1, err := sch.first.Decapsulate(priv.first, ct[:firstSize])
	if err != nil {
		return nil, err
	}
	ss2, err := sch.second.Decapsulate(priv.second, ct[firstSize:])
	if err != nil {
		return nil, err
	}

	return util.PairSplitPRF(ss1, ss2, ct[:firstSize], ct[firstSize:]), nil
}

func (sch *Scheme) UnmarshalBinaryPublicKey(buf []byte) (kem.PublicKey, error) {
	if len(buf) != sch.PublicKeySize() {
		return nil, kem.ErrPubKeySize
	}
	firstSize := sch.first.PublicKeySize()
	pk1, err := sch.first.UnmarshalBinaryPublicKey(buf[:firstSize])
	if err != nil {
		return nil, err
	}
	pk2, err := sch.second.UnmarshalBinaryPublicKey(buf[firstSize:])
	if err != nil {
		return nil, err
	}
	return &PublicKey{sch, pk1, pk2}, nil
}

func (sch *Scheme) UnmarshalBinaryPrivateKey(buf []byte) (kem.PrivateKey, error) {
	if len(buf) != sch.PrivateKeySize() {
		return nil, kem.ErrPrivKeySize
	}
	firstSize := sch.first.PrivateKeySize()
	sk1, err := sch.first.UnmarshalBinaryPrivateKey(buf[:firstSize])
	if err != nil {
		return nil, err
	}
	sk2, err := sch.second.UnmarshalBinaryPrivateKey(buf[firstSize:])
	if err != nil {
		return nil, err
	}
	return &PrivateKey{sch, sk1, sk2}, nil
}
