package sign

import (
	"reflect"

	"encoding/json"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/hashid"
	"github.com/dedis/cothority/lib/proof"
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/suites"
	"github.com/dedis/protobuf"
)

/*
All message structures defined in this package are used in the
Collective Signing Protocol
Over the network they are sent as byte slices, so each message
has its own MarshalBinary and UnmarshalBinary method
*/

type MessageType int

const (
	Unset MessageType = iota
	Announcement
	Commitment
	Challenge
	Response
	SignatureBroadcast
	StatusReturn
	CatchUpReq
	CatchUpResp
	GroupChange
	GroupChanged
	StatusConnections
	CloseAll
	Default // for internal use
	Error
)

func (m MessageType) String() string {
	switch m {
	case Unset:
		return "Unset"
	case Announcement:
		return "Announcement"
	case Commitment:
		return "Commitment"
	case Challenge:
		return "Challenge"
	case Response:
		return "Response"
	case SignatureBroadcast:
		return "SignatureBroadcast"
	case StatusReturn:
		return "StatusReturn"
	case CatchUpReq:
		return "CatchUpRequest"
	case CatchUpResp:
		return "CatchUpResponse"
	case GroupChange:
		return "GroupChange"
	case GroupChanged:
		return "GroupChanged"
	case StatusConnections:
		return "StatusConnections"
	case CloseAll:
		return "CloseAll"
	case Default: // for internal use
		return "Default"
	case Error:
		return "Error"
	}
	return "INVALID TYPE"
}

// Signing Messages are used for all communications between servers
// It is important for encoding/ decoding for type to be kept as first field
type SigningMessage struct {
	Suite        string
	Type         MessageType
	Am           *AnnouncementMessage
	Com          *CommitmentMessage
	Chm          *ChallengeMessage
	Rm           *ResponseMessage
	SBm          *SignatureBroadcastMessage
	SRm          *StatusReturnMessage
	Cureq        *CatchUpRequest
	Curesp       *CatchUpResponse
	Vrm          *VoteRequestMessage
	Gcm          *GroupChangedMessage
	Err          *ErrorMessage
	From         string
	To           string
	ViewNbr      int
	LastSeenVote int // highest vote ever seen and commited in log, used for catch-up
	RoundNbr     int
}

// Helper functions that will return the suite used during the process from a string name
func GetSuite(suite string) abstract.Suite {
	s, ok := suites.All()[suite]
	if !ok {
		dbg.Lvl1("Suites available:", suites.All())
		dbg.Fatal("Didn't find suite", suite)
	}
	return s
}

func NewSigningMessage() interface{} {
	return &SigningMessage{}
}

func (sm *SigningMessage) MarshalBinary() ([]byte, error) {
	b, e := protobuf.Encode(sm)
	if len(b) != 0 {
		//dbg.Print("Length of bytes is", len(b), "for", sm)
		//debug.PrintStack()
	}
	return b, e
}

func (sm *SigningMessage) UnmarshalBinary(data []byte) error {
	dbg.Fatal("Shouldn't be called")
	return nil
}

func (sm *SigningMessage) UnmarshalBinarySuite(jdata *JSONdata) error {
	suite := GetSuite(jdata.Suite)
	var cons = make(protobuf.Constructors)
	var point abstract.Point
	var secret abstract.Secret
	cons[reflect.TypeOf(&point).Elem()] = func() interface{} { return suite.Point() }
	cons[reflect.TypeOf(&secret).Elem()] = func() interface{} { return suite.Secret() }
	return protobuf.DecodeWithConstructors(jdata.Data, sm, cons)
}

type JSONdata struct {
	Suite string
	Data  []byte
}

func (sm *SigningMessage) MarshalJSON() ([]byte, error) {
	data, err := sm.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return json.Marshal(JSONdata{
		Suite: sm.Suite,
		Data:  data,
	})
}

func (sm *SigningMessage) UnmarshalJSON(dataJSON []byte) error {
	jdata := &JSONdata{}
	json.Unmarshal(dataJSON, jdata)
	return sm.UnmarshalBinarySuite(jdata)
}

// Broadcasted message initiated and signed by proposer
type AnnouncementMessage struct {
	Message   []byte
	RoundType string // what kind of round this announcement is made for
	// VoteRequest *VoteRequest
	Vote *Vote // Vote Request (propose)
}

// Commitment of all nodes together with the data they want
// to have signed
type CommitmentMessage struct {
	Message []byte
	V       abstract.Point // commitment Point
	V_hat   abstract.Point // product of subtree participating nodes' commitment points
	X_hat   abstract.Point // product of subtree participating nodes' public keys

	MTRoot hashid.HashId // root of Merkle (sub)Tree

	// public keys of children servers that did not respond to
	// annoucement from root
	RejectionPublicList []abstract.Point

	// CountedVotes *CountedVotes // CountedVotes contains a subtree's votes
	Vote *Vote // Vote Response (promise)

	Messages int // Actual number of messages signed
}

// The challenge calculated by the root-node
type ChallengeMessage struct {
	Message []byte
	C       abstract.Secret // challenge

	// Depth  byte
	MTRoot hashid.HashId // the very root of the big Merkle Tree
	Proof  proof.Proof   // Merkle Path of Proofs from root to us

	// CountedVotes *CountedVotes //  CountedVotes contains the whole tree's votes
	Vote *Vote // Vote Confirmerd/ Rejected (accept)

}

// Every node replies with eventual exceptions if they
// are not OK
type ResponseMessage struct {
	Message []byte
	R_hat   abstract.Secret // response

	// public keys of children servers that did not respond to
	// challenge from root
	RejectionPublicList []abstract.Point
	// nodes that refused to commit:
	RejectionCommitList []abstract.Point

	// cummulative point commits of nodes that failed after commit
	ExceptionV_hat abstract.Point
	// cummulative public keys of nodes that failed after commit
	ExceptionX_hat abstract.Point

	Vote *Vote // Vote Ack/Nack in thr log (ack/nack)

}

// 5th message going from root to leaves to send the
// signature
type SignatureBroadcastMessage struct {
	// Aggregate response of root
	R0_hat abstract.Secret
	// Challenge
	C abstract.Secret
	// Aggregate public key
	X0_hat abstract.Point
	// Aggregate public commitment
	V0_hat abstract.Point
	// challenge from root
	RejectionPublicList []abstract.Point
	RejectionCommitList []abstract.Point
	// Number of messages signed
	Messages int
}

// StatusReturnMessage carries the last status after the
// SignatureBroadcastMessage has been sent to everybody.
// Every node should just add up the stats from its children.
type StatusReturnMessage struct {
	// How many nodes sent a 'respond' message
	Responders int
	// How many peers contacted for a challenge
	Peers int
}

// In case of an error, this message is sent
type ErrorMessage struct {
	Err string
}

// For request of a vote on tree-structure change
type VoteRequestMessage struct {
	Vote *Vote
}

// Whenever the group changed
type GroupChangedMessage struct {
	V *Vote
	// if vote not accepted rest of fields are nil
	HostList []string
}
