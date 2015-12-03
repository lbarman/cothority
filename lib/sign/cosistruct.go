package sign

import (
	"bytes"
	"encoding/gob"
	"errors"
	"github.com/dedis/cothority/lib/coconet"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/hashid"
	"github.com/dedis/cothority/lib/proof"
	"github.com/dedis/crypto/abstract"
	"sort"
)

/*
Functionality used in the roundcosi. Abstracted here for better
understanding and readability of roundcosi.
*/

const FIRST_ROUND int = 1 // start counting rounds at 1

type CosiStruct struct {
	// Message created by root. It can be empty and it will make no difference. In
	// the case of a timestamp service however we need the timestamp generated by
	// the round for this round . It will be included in the challenge, and then
	// can be verified by the client
	Msg []byte
	C   abstract.Secret // round lasting challenge
	R   abstract.Secret // round lasting response

	Log       SNLog // round lasting log structure
	HashedLog []byte

	R_hat abstract.Secret // aggregate of responses

	X_hat abstract.Point // aggregate of public keys

	Commits   []*SigningMessage
	Responses []*SigningMessage

	// own big merkle subtree
	MTRoot     hashid.HashId   // mt root for subtree, passed upwards
	Leaves     []hashid.HashId // leaves used to build the merkle subtre
	LeavesFrom []string        // child names for leaves

	// mtRoot before adding HashedLog
	LocalMTRoot hashid.HashId

	// merkle tree roots of children in strict order
	CMTRoots     []hashid.HashId
	CMTRootNames []string
	Proofs       map[string]proof.Proof
	Proof        []hashid.HashId
	PubKey       abstract.Point
	PrivKey      abstract.Secret
	Name         string

	// round-lasting public keys of children servers that did not
	// respond to latest commit or respond phase, in subtree
	ExceptionList []abstract.Point
	// combined point commits of children servers in subtree
	ChildV_hat map[string]abstract.Point
	// combined public keys of children servers in subtree
	ChildX_hat map[string]abstract.Point
	// for internal verification purposes
	ExceptionX_hat abstract.Point
	ExceptionV_hat abstract.Point

	BackLink hashid.HashId
	AccRound []byte

	Suite abstract.Suite

	Children map[string]coconet.Conn
	Parent   string
	ViewNbr  int
}

// Sets up a round according to the needs stated in the
// Announcementmessage.
func NewCosi(sn *Node, viewNbr, roundNbr int, am *AnnouncementMessage) *CosiStruct {
	// set up commit and response channels for the new round
	cosi := &CosiStruct{}
	cosi.Commits = make([]*SigningMessage, 0)
	cosi.Responses = make([]*SigningMessage, 0)
	cosi.ExceptionList = make([]abstract.Point, 0)
	cosi.Suite = sn.suite
	cosi.Log.Suite = sn.suite
	cosi.Children = sn.Children(viewNbr)
	cosi.Parent = sn.Parent(viewNbr)
	cosi.ViewNbr = viewNbr
	cosi.PubKey = sn.PubKey
	cosi.PrivKey = sn.PrivKey
	cosi.Name = sn.Name()
	cosi.ExceptionV_hat = sn.suite.Point().Null()
	cosi.ExceptionX_hat = sn.suite.Point().Null()
	cosi.ExceptionList = make([]abstract.Point, 0)
	cosi.InitCommitCrypto()
	return cosi
}

/*
 * This is a module for the round-struct that does all the
 * calculation for a merkle-hash-tree.
 */

// Create round lasting secret and commit point v and V
// Initialize log structure for the round
func (cosi *CosiStruct) InitCommitCrypto() {
	// generate secret and point commitment for this round
	rand := cosi.Suite.Cipher([]byte(cosi.Name))
	cosi.Log = SNLog{}
	cosi.Log.v = cosi.Suite.Secret().Pick(rand)
	cosi.Log.V = cosi.Suite.Point().Mul(nil, cosi.Log.v)
	// initialize product of point commitments
	cosi.Log.V_hat = cosi.Suite.Point().Null()
	cosi.Log.Suite = cosi.Suite
	//cosi.Add(cosi.Log.V_hat, cosi.Log.V)
	cosi.Log.V_hat.Add(cosi.Log.V_hat, cosi.Log.V)

	cosi.X_hat = cosi.Suite.Point().Null()
	//cosi.Add(cosi.X_hat, cosi.PubKey)
	cosi.X_hat.Add(cosi.X_hat, cosi.PubKey)
}

// Adds a child-node to the Merkle-tree and updates the root-hashes
func (cosi *CosiStruct) MerkleAddChildren() {
	// children commit roots
	cosi.CMTRoots = make([]hashid.HashId, len(cosi.Leaves))
	copy(cosi.CMTRoots, cosi.Leaves)
	cosi.CMTRootNames = make([]string, len(cosi.Leaves))
	copy(cosi.CMTRootNames, cosi.LeavesFrom)

	// concatenate children commit roots in one binary blob for easy marshalling
	cosi.Log.CMTRoots = make([]byte, 0)
	for _, leaf := range cosi.Leaves {
		cosi.Log.CMTRoots = append(cosi.Log.CMTRoots, leaf...)
	}
}

// Adds the local Merkle-tree root, usually from a stamper or
// such
func (cosi *CosiStruct) MerkleAddLocal(localMTroot hashid.HashId) {
	// add own local mtroot to leaves
	cosi.LocalMTRoot = localMTroot
	cosi.Leaves = append(cosi.Leaves, cosi.LocalMTRoot)
}

// Hashes the log of the round-structure
func (cosi *CosiStruct) MerkleHashLog() error {
	var err error

	h := cosi.Suite.Hash()
	logBytes, err := cosi.Log.MarshalBinary()
	if err != nil {
		return err
	}
	h.Write(logBytes)
	cosi.HashedLog = h.Sum(nil)
	return err
}

func (cosi *CosiStruct) ComputeCombinedMerkleRoot() {
	// add hash of whole log to leaves
	cosi.Leaves = append(cosi.Leaves, cosi.HashedLog)

	// compute MT root based on Log as right child and
	// MT of leaves as left child and send it up to parent
	sort.Sort(hashid.ByHashId(cosi.Leaves))
	left, proofs := proof.ProofTree(cosi.Suite.Hash, cosi.Leaves)
	right := cosi.HashedLog
	moreLeaves := make([]hashid.HashId, 0)
	moreLeaves = append(moreLeaves, left, right)
	cosi.MTRoot, _ = proof.ProofTree(cosi.Suite.Hash, moreLeaves)

	// Hashed Log has to come first in the proof; len(sn.CMTRoots)+1 proofs
	cosi.Proofs = make(map[string]proof.Proof, 0)
	for name := range cosi.Children {
		cosi.Proofs[name] = append(cosi.Proofs[name], right)
	}
	cosi.Proofs["local"] = append(cosi.Proofs["local"], right)

	// separate proofs by children (need to send personalized proofs to children)
	// also separate local proof (need to send it to timestamp server)
	cosi.SeparateProofs(proofs, cosi.Leaves)
}

// Identify which proof corresponds to which leaf
// Needed given that the leaves are sorted before passed to the function that create
// the Merkle Tree and its Proofs
func (cosi *CosiStruct) SeparateProofs(proofs []proof.Proof, leaves []hashid.HashId) {
	// separate proofs for children servers mt roots
	for i := 0; i < len(cosi.CMTRoots); i++ {
		name := cosi.CMTRootNames[i]
		for j := 0; j < len(leaves); j++ {
			if bytes.Compare(cosi.CMTRoots[i], leaves[j]) == 0 {
				// sn.Proofs[i] = append(sn.Proofs[i], proofs[j]...)
				cosi.Proofs[name] = append(cosi.Proofs[name], proofs[j]...)
				continue
			}
		}
	}

	// separate proof for local mt root
	for j := 0; j < len(leaves); j++ {
		if bytes.Compare(cosi.LocalMTRoot, leaves[j]) == 0 {
			cosi.Proofs["local"] = append(cosi.Proofs["local"], proofs[j]...)
		}
	}
}

func (cosi *CosiStruct) InitResponseCrypto() {
	cosi.R = cosi.Suite.Secret()
	cosi.R.Mul(cosi.PrivKey, cosi.C).Sub(cosi.Log.v, cosi.R)
	// initialize sum of children's responses
	cosi.R_hat = cosi.R
}

// Create Merkle Proof for local client (timestamp server) and
// store it in Node so that we can send it to the clients during
// the SignatureBroadcast
func (cosi *CosiStruct) StoreLocalMerkleProof(chm *ChallengeMessage) error {
	proofForClient := make(proof.Proof, len(chm.Proof))
	copy(proofForClient, chm.Proof)

	// To the proof from our root to big root we must add the separated proof
	// from the localMKT of the client (timestamp server) to our root
	proofForClient = append(proofForClient, cosi.Proofs["local"]...)

	// if want to verify partial and full proofs
	if dbg.DebugVisible > 2 {
		//round.sn.VerifyAllProofs(view, chm, proofForClient)
	}
	cosi.Proof = proofForClient
	cosi.MTRoot = chm.MTRoot
	return nil
}

// Called by every node after receiving aggregate responses from descendants
func (cosi *CosiStruct) VerifyResponses() error {

	// Check that: base**r_hat * X_hat**c == V_hat
	// Equivalent to base**(r+xc) == base**(v) == T in vanillaElGamal
	Aux := cosi.Suite.Point()
	V_clean := cosi.Suite.Point()
	V_clean.Add(V_clean.Mul(nil, cosi.R_hat), Aux.Mul(cosi.X_hat, cosi.C))
	// T is the recreated V_hat
	T := cosi.Suite.Point().Null()
	T.Add(T, V_clean)
	T.Add(T, cosi.ExceptionV_hat)

	var c2 abstract.Secret
	isroot := cosi.Parent == ""
	if isroot {
		// round challenge must be recomputed given potential
		// exception list
		msg := cosi.Msg
		msg = append(msg, []byte(cosi.MTRoot)...)
		cosi.C = cosi.HashElGamal(msg, cosi.Log.V_hat)
		c2 = cosi.HashElGamal(msg, T)
	}

	// intermediary nodes check partial responses aginst their partial keys
	// the root node is also able to check against the challenge it emitted
	if !T.Equal(cosi.Log.V_hat) || (isroot && !cosi.C.Equal(c2)) {
		return errors.New("Verifying ElGamal Collective Signature failed in " +
			cosi.Name)
	} else if isroot {
		dbg.Lvl4(cosi.Name, "reports ElGamal Collective Signature succeeded")
	}
	return nil
}

// Returns a secret that depends on on a message and a point
func (cosi *CosiStruct) HashElGamal(message []byte, p abstract.Point) abstract.Secret {
	pb, _ := p.MarshalBinary()
	c := cosi.Suite.Cipher(pb)
	c.Message(nil, nil, message)
	return cosi.Suite.Secret().Pick(c)
}

// Signing Node Log for a round
// For Marshaling and Unmarshaling to work smoothly
// crypto fields must appear first in the structure
type SNLog struct {
	v     abstract.Secret // round lasting secret
	V     abstract.Point  // round lasting commitment point
	V_hat abstract.Point  // aggregate of commit points

	// merkle tree roots of children in strict order
	CMTRoots hashid.HashId // concatenated hash ids of children
	Suite    abstract.Suite
}

func (snLog SNLog) MarshalBinary() ([]byte, error) {
	// abstract.Write used to encode/ marshal crypto types
	b := bytes.Buffer{}
	snLog.Suite.Write(&b, &snLog.v, &snLog.V, &snLog.V_hat)
	////// gob is used to encode non-crypto types
	enc := gob.NewEncoder(&b)
	err := enc.Encode(snLog.CMTRoots)
	return b.Bytes(), err
}

func (snLog *SNLog) UnmarshalBinary(data []byte) error {
	// abstract.Read used to decode/ unmarshal crypto types
	b := bytes.NewBuffer(data)
	err := snLog.Suite.Read(b, &snLog.v, &snLog.V, &snLog.V_hat)
	// gob is used to decode non-crypto types
	rem, _ := snLog.MarshalBinary()
	snLog.CMTRoots = data[len(rem):]
	return err
}

func (snLog *SNLog) Getv() abstract.Secret {
	return snLog.v
}
