package randhound

import (
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/random"
)

func (rh *RandHound) hash(bytes ...[]byte) []byte {
	return abstract.Sum(rh.Node.Suite(), bytes...)
}

func (rh *RandHound) chooseInsurers(Rc, Rs []byte) ([]int, []abstract.Point) {

	// Seed PRNG for insurers selection
	var seed []byte
	seed = append(seed, Rc...)
	seed = append(seed, Rs...)
	prng := rh.Node.Suite().Cipher(seed)

	// Choose insurers uniquely
	set := make(map[int]bool)
	insurers := make([]abstract.Point, rh.N)
	keys := make([]int, rh.N)
	tns := rh.Tree().ListNodes()
	j := 0
	for len(set) < rh.N {
		i := int(random.Uint64(prng) % uint64(len(tns)))
		// Add insurer only if not done so before; choosing yourself as an insurer is fine; ignore leader at index 0
		if _, ok := set[i]; !ok && !tns[i].IsRoot() {
			set[i] = true
			keys[j] = i - 1
			insurers[j] = tns[i].Entity.Public
			j += 1
		}
	}
	return keys, insurers
}