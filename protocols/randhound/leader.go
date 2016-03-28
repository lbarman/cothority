package randhound

import "github.com/dedis/crypto/poly"

type Leader struct {
	Rc     []byte                  // Leader's trustee-selection random value
	Rs     [][]byte                // Peers' trustee-selection random values
	i1     I1                      // I1 message sent to the peers
	i2     I2                      // I2 - " -
	i3     I3                      // I3 - " -
	i4     I4                      // I4 - " -
	r1     map[int]*R1             // R1 messages received from the peers
	r2     map[int]*R2             // R2 - " -
	r3     map[int]*R3             // R3 - " -
	r4     map[int]*R4             // R4 - " -
	deals  map[int]*poly.Deal      // Unmarshaled deals from peers
	shares map[int]*poly.PriShares // Revealed shares
	Done   chan bool               // For signaling that a protocol run is finished
	Result chan []byte             // For returning the generated randomness
}

func (rh *RandHound) newLeader() (*Leader, error) {
	return &Leader{
		r1:     make(map[int]*R1),
		r2:     make(map[int]*R2),
		r3:     make(map[int]*R3),
		r4:     make(map[int]*R4),
		deals:  make(map[int]*poly.Deal),
		shares: make(map[int]*poly.PriShares),
		Done:   make(chan bool, 1),
		Result: make(chan []byte),
	}, nil
}