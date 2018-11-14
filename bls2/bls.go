package bls2

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
)

const BYTES_REPR_SIZE = FP256BN.MODBYTES * 4

type SignKey struct {
	groupOrderElement *groupOrderElement
}

type VerKey struct {
	point *PointG2
}

type Generator struct {
	point *PointG2
	bytes []byte
}

type Signature struct {
	point *PointG1
}

type MultiSignature struct {
	point *PointG1
}

type PointG2 struct {
	point *FP256BN.ECP2
}

type PointG1 struct {
	point *FP256BN.ECP
}

type Pair struct {
	pair *FP256BN.FP12
}

type groupOrderElement struct {
	bn *FP256BN.BIG
}

func newPointG2() *PointG2 {
	pointXa := FP256BN.NewBIGints(FP256BN.CURVE_Pxa)
	pointXb := FP256BN.NewBIGints(FP256BN.CURVE_Pxb)
	pointYa := FP256BN.NewBIGints(FP256BN.CURVE_Pya)
	pointYb := FP256BN.NewBIGints(FP256BN.CURVE_Pyb)

	pointX := FP256BN.NewFP2bigs(pointXa, pointXb)
	pointY := FP256BN.NewFP2bigs(pointYa, pointYb)
	genG2 := FP256BN.NewECP2fp2s(pointX, pointY)
	point := FP256BN.G2mul(genG2, randomModOrder())
	return &PointG2{
		point: point,
	}
}

func (g2 *PointG2) Mul(e *groupOrderElement) *PointG2 {
	return &PointG2{
		point: FP256BN.G2mul(g2.point, e.bn),
	}
}

func (g2 *PointG2) Add(other *PointG2) {
	g2.point.Add(other.point)
}

func (g1 *PointG1) Mul(e *groupOrderElement) *PointG1 {
	return &PointG1{
		point: FP256BN.G1mul(g1.point, e.bn),
	}
}

func (g1 *PointG1) Add(other *PointG1) {
	g1.point.Add(other.point)
}

func NewInfPointG1() *PointG1 {
	p := FP256BN.NewECP()
	if !p.Is_infinity() {
		panic("non infinite point!")
	}
	return &PointG1{
		point: p,
	}
}

func NewInfPointG2() *PointG2 {
	p := FP256BN.NewECP2()
	if !p.Is_infinity() {
		panic("non infinite point!")
	}
	return &PointG2{
		point: p,
	}
}

func PairPoints(p *PointG1, q *PointG2) *Pair {
	return &Pair{
		pair: FP256BN.Fexp(FP256BN.Ate(q.point, p.point)),
	}
}

func (p *Pair) Eq(other *Pair) bool {
	return p.pair.Equals(other.pair)
}

func newVerKey(gen *Generator, sk *SignKey) *VerKey {
	point := gen.point.Mul(sk.groupOrderElement)
	return &VerKey{
		point: point,
	}
}

func NewMultiSignature(sigs []*Signature) *MultiSignature {
	point := NewInfPointG1()
	for _, sig := range sigs {
		point.Add(sig.point)
	}
	return &MultiSignature{
		point: point,
	}
}

func randomModOrder() *FP256BN.BIG {
	entropy := make([]byte, 128)
	rand.Read(entropy)
	rng := amcl.NewRAND()
	rng.Seed(128, entropy)
	return FP256BN.Randomnum(FP256BN.NewBIGints(FP256BN.CURVE_Order), rng)
}

func newGroupOrderElement() *groupOrderElement {
	return &groupOrderElement{
		bn: randomModOrder(),
	}
}

func groupOrderElementFromBytes(bits []byte) (*groupOrderElement, error) {
	if uint(len(bits)) > BYTES_REPR_SIZE {
		return nil, fmt.Errorf("invalid len of bytes representation")
	}

	if uint(len(bits)) < FP256BN.MODBYTES {
		diff := FP256BN.MODBYTES - uint(len(bits))
		bits = append(make([]byte, diff), bits...)
	}

	return &groupOrderElement{
		bn: FP256BN.FromBytes(bits),
	}, nil
}

func NewSignKey() *SignKey {
	return &SignKey{
		groupOrderElement: newGroupOrderElement(),
	}
}

func NewGenerator() *Generator {
	point := newPointG2()
	bits := make([]byte, BYTES_REPR_SIZE)
	point.point.ToBytes(bits)
	return &Generator{
		point: newPointG2(),
		bytes: bits,
	}
}

func Sign(msg []byte, signKey *SignKey) *Signature {
	return &Signature{
		point: hash(msg).Mul(signKey.groupOrderElement),
	}
}

func VerifySignature(sig *Signature, msg []byte, verKey *VerKey, gen *Generator) bool {
	return verifySignature(sig.point, msg, verKey.point, gen)
}

func VerifyMultiSignature(sig *MultiSignature, msg []byte, verKeys []*VerKey, gen *Generator) bool {
	aggregatedVerKey := NewInfPointG2()
	for _, verKey := range verKeys {
		aggregatedVerKey.Add(verKey.point)
	}
	return verifySignature(sig.point, msg, aggregatedVerKey, gen)
}

func verifySignature(sig *PointG1, msg []byte, verKey *PointG2, gen *Generator) bool {
	h := hash(msg)
	return PairPoints(sig, gen.point).Eq(PairPoints(h, verKey))
}

func hash(msg []byte) *PointG1 {
	hsh := sha256.New()
	hsh.Write(msg)
	bits := hsh.Sum(nil)
	el, err := groupOrderElementFromBytes(bits)
	if err != nil {
		panic("invalid code, not user problem")
	}
	bn := el.bn

	point := FP256BN.NewECPbig(bn)
	for point.Is_infinity() {
		bn = bn.Plus(FP256BN.NewBIGint(1))
		point = FP256BN.NewECPbig(bn)
	}
	return &PointG1{
		point: point,
	}
}
