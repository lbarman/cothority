// better not get sda_test, cannot access unexported fields
package sda_test

import (
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/network"
	"github.com/dedis/cothority/lib/sda"
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/config"
	"github.com/dedis/crypto/random"
	"github.com/satori/go.uuid"
	"testing"
	"time"
)

var suite abstract.Suite = network.Suite

func init() {
	network.RegisterProtocolType(SimpleMessageType, SimpleMessage{})
}

// Test setting up of Host
func TestHostNew(t *testing.T) {
	h1 := newHost("localhost:2000", suite)
	if h1 == nil {
		t.Fatal("Couldn't setup a Host")
	}
	err := h1.Close()
	if err != nil {
		t.Fatal("Couldn't close", err)
	}
}

// Test closing and opening of Host on same address
func TestHostClose(t *testing.T) {
	dbg.TestOutput(testing.Verbose(), 4)
	h1 := newHost("localhost:2000", suite)
	h2 := newHost("localhost:2001", suite)
	h1.Listen()
	h2.Connect(h1.Identity)
	err := h1.Close()
	if err != nil {
		t.Fatal("Couldn't close:", err)
	}
	err = h2.Close()
	if err != nil {
		t.Fatal("Couldn't close:", err)
	}
	dbg.Lvl3("Finished first connection, starting 2nd")
	h1 = newHost("localhost:2002", suite)
	h1.Listen()
	if err != nil {
		t.Fatal("Couldn't re-open listener")
	}
	dbg.Lvl3("Closing h1")
	h1.Close()
}

// Test connection of multiple Hosts and sending messages back and forth
func TestHostMessaging(t *testing.T) {
	h1, h2 := setupHosts(t, false)
	msgSimple := &SimpleMessage{3}
	err := h1.SendMsgTo(h2.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send from h2 -> h1:", err)
	}
	msg := h2.Receive().Data
	decoded := testMessageSimple(t, msg)
	if decoded.I != 3 {
		t.Fatal("Received message from h2 -> h1 is wrong")
	}

	h1.Close()
	h2.Close()
}

// Test parsing of incoming packets with regard to its double-included
// data-structure
func TestHostIncomingMessage(t *testing.T) {
	h1, h2 := setupHosts(t, false)
	msgSimple := &SimpleMessage{10}
	err := h1.SendMsgTo(h2.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send message:", err)
	}

	msg := h2.Receive().Data
	decoded := testMessageSimple(t, msg)
	if decoded.I != 10 {
		t.Fatal("Wrong value")
	}

	h1.Close()
	h2.Close()
}

// Test sending data back and forth using the SendMsgTo
func TestHostSendMsgDuplex(t *testing.T) {
	h1, h2 := setupHosts(t, false)
	msgSimple := &SimpleMessage{5}
	err := h1.SendMsgTo(h2.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send message from h1 to h2", err)
	}
	msg := h2.Receive()
	dbg.Lvl2("Received msg h1 -> h2", msg)

	err = h2.SendMsgTo(h1.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send message from h2 to h1", err)
	}
	msg = h1.Receive()
	dbg.Lvl2("Received msg h2 -> h1", msg)

	h1.Close()
	h2.Close()
}

// Test sending data back and forth using the SendTo
func TestHostSendDuplex(t *testing.T) {
	h1, h2 := setupHosts(t, false)
	msgSimple := &SimpleMessage{5}
	err := h1.SendToRaw(h2.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send message from h1 to h2", err)
	}
	msg := h2.Receive()
	dbg.Lvl2("Received msg h1 -> h2", msg)

	err = h2.SendToRaw(h1.Identity, msgSimple)
	if err != nil {
		t.Fatal("Couldn't send message from h2 to h1", err)
	}
	msg = h1.Receive()
	dbg.Lvl2("Received msg h2 -> h1", msg)

	h1.Close()
	h2.Close()
}

// Test propagation of peer-lists - both known and unknown
func TestPeerListPropagation(t *testing.T) {
	h1, h2 := setupHosts(t, true)
	il1 := GenIdentityList(h1.Suite(), genLocalhostPeerNames(10, 2000))

	// Check that h2 sends back an empty list if it is unknown
	err := h1.SendToRaw(h2.Identity, &sda.RequestIdentityList{il1.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	msg := h1.Receive().Data
	if msg.MsgType != sda.SendIdentityListMessage {
		t.Fatal("h1 didn't receive IdentityList type, but", msg.MsgType)
	}
	if msg.Msg.(sda.IdentityList).Id != uuid.Nil {
		t.Fatal("List should be empty")
	}

	// Now add the list to h2 and try again
	h2.AddIdentityList(il1)
	err = h1.SendToRaw(h2.Identity, &sda.RequestIdentityList{il1.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	msg = h1.Receive().Data
	if msg.MsgType != sda.SendIdentityListMessage {
		t.Fatal("h1 didn't receive IdentityList type")
	}
	if msg.Msg.(sda.IdentityList).Id != il1.Id {
		t.Fatal("List should be equal to original list")
	}

	// And test whether it gets stored correctly
	go h1.ProcessMessages()
	err = h1.SendToRaw(h2.Identity, &sda.RequestIdentityList{il1.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	time.Sleep(time.Second)
	list, ok := h1.GetIdentityList(il1.Id)
	if !ok {
		t.Fatal("List-id not found")
	}
	if list.Id != il1.Id {
		t.Fatal("IDs do not match")
	}
	h1.Close()
	h2.Close()
}

// Test propagation of tree - both known and unknown
func TestTreePropagation(t *testing.T) {
	h1, h2 := setupHosts(t, true)
	il1 := GenIdentityList(h1.Suite(), genLocalhostPeerNames(10, 2000))
	// Suppose both hosts have the list available, but not the tree
	h1.AddIdentityList(il1)
	h2.AddIdentityList(il1)
	tree, _ := GenerateTreeFromIdentityList(il1)

	// Check that h2 sends back an empty tree if it is unknown
	err := h1.SendToRaw(h2.Identity, &sda.RequestTree{tree.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	msg := h1.Receive().Data
	if msg.MsgType != sda.SendTreeMessage {
		t.Fatal("h1 didn't receive SendTree type:", msg.MsgType)
	}
	if msg.Msg.(sda.TreeMarshal).Identity != uuid.Nil {
		t.Fatal("List should be empty")
	}

	// Now add the list to h2 and try again
	h2.AddTree(tree)
	err = h1.SendToRaw(h2.Identity, &sda.RequestTree{tree.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	msg = h1.Receive().Data
	if msg.MsgType != sda.SendTreeMessage {
		t.Fatal("h1 didn't receive Tree-type")
	}
	if msg.Msg.(sda.TreeMarshal).Node != tree.Id {
		t.Fatal("Tree should be equal to original tree")
	}

	// And test whether it gets stored correctly
	go h1.ProcessMessages()
	err = h1.SendToRaw(h2.Identity, &sda.RequestTree{tree.Id})
	if err != nil {
		t.Fatal("Couldn't send message to h2:", err)
	}
	time.Sleep(time.Second)
	tree2, ok := h1.GetTree(tree.Id)
	if !ok {
		t.Fatal("List-id not found")
	}
	if !tree.Equal(tree2) {
		t.Fatal("Trees do not match")
	}
}

// Tests both list- and tree-propagation
func TestListTreePropagation(t *testing.T) {

}

// Test instantiation of ProtocolInstances

// Test access of actual peer that received the message
// - corner-case: accessing parent/children with multiple instances of the same peer
// in the graph - ProtocolID + GraphID + InstanceID is not enough
// XXX ???

// Test complete parsing of new incoming packet
// - Test if it is SDAMessage
// - reject if unknown ProtocolID
// - setting up of graph and Hostlist
// - instantiating ProtocolInstance

// privPub creates a private/public key pair
func privPub(s abstract.Suite) (abstract.Secret, abstract.Point) {
	keypair := &config.KeyPair{}
	keypair.Gen(s, random.Stream)
	return keypair.Secret, keypair.Public
}

func newHost(address string, s abstract.Suite) *sda.Host {
	priv, pub := privPub(s)
	id := &network.Entity{Public: pub, Addresses: []string{address}}
	return sda.NewHost(id, priv, network.NewSecureTcpHost(priv, id))
}

// Creates two hosts on the local interfaces,
func setupHosts(t *testing.T, h2process bool) (*sda.Host, *sda.Host) {
	dbg.TestOutput(testing.Verbose(), 4)
	h1 := newHost("localhost:2000", suite)
	// make the second peer as the server
	h2 := newHost("localhost:2001", suite)
	h2.Listen()
	// make it process messages
	if h2process {
		go h2.ProcessMessages()
	}
	_, err := h1.Connect(h2.Identity)
	if err != nil {
		t.Fatal(err)
	}
	return h1, h2
}

// SimpleMessage is just used to transfer one integer
type SimpleMessage struct {
	I int
}

const (
	SimpleMessageType = iota + 50
)

func testMessageSimple(t *testing.T, msg network.ApplicationMessage) SimpleMessage {
	if msg.MsgType != sda.SDADataMessage {
		t.Fatal("Wrong message type received:", msg.MsgType)
	}
	sda := msg.Msg.(sda.SDAData)
	if sda.MsgType != SimpleMessageType {
		t.Fatal("Couldn't pass simple message")
	}
	return sda.Msg.(SimpleMessage)
}