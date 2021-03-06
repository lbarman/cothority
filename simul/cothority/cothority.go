// The simulation cothority used for all protocols.
// This should not be used stand-alone and is only for
// the simulations. It loads the simulation-file, initialises all
// necessary hosts and starts the simulation on the root-node.
package main

import (
	"flag"

	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/sda"
	"time"

	"github.com/dedis/cothority/lib/monitor"
	"github.com/dedis/cothority/protocols/manage"
	// Empty imports to have the init-functions called which should
	// register the protocol
	_ "github.com/dedis/cothority/protocols"
)

// The address of this host - if there is only one host in the config
// file, it will be derived from it automatically
var hostAddress string

// ip addr of the logger to connect to
var monitorAddress string

// Simul is != "" if this node needs to start a simulation of that protocol
var simul string

var debugVisible int

// Initialize before 'init' so we can directly use the fields as parameters
// to 'Flag'
func init() {
	flag.StringVar(&hostAddress, "address", "", "our address to use")
	flag.StringVar(&simul, "simul", "", "start simulating that protocol")
	flag.StringVar(&monitorAddress, "monitor", "", "remote monitor")
	flag.IntVar(&debugVisible, "debug", 1, "verbosity: 0-5")
}

// Main starts the host and will setup the protocol.
func main() {
	flag.Parse()
	dbg.SetDebugVisible(debugVisible)
	dbg.Lvl3("Flags are:", hostAddress, simul, dbg.DebugVisible, monitorAddress)

	scs, err := sda.LoadSimulationConfig(".", hostAddress)
	if err != nil {
		// We probably are not needed
		dbg.Lvl2(err)
		return
	}
	if monitorAddress != "" {
		monitor.ConnectSink(monitorAddress)
	}
	sims := make([]sda.Simulation, len(scs))
	var rootSC *sda.SimulationConfig
	var rootSim sda.Simulation
	for i, sc := range scs {
		// Starting all hosts for that server
		host := sc.Host
		dbg.Lvl3(hostAddress, "Starting host", host.Entity.Addresses)
		host.ListenNoblock()
		host.StartProcessMessages()
		sim, err := sda.NewSimulation(simul, sc.Config)
		if err != nil {
			dbg.Fatal(err)
		}
		err = sim.Node(sc)
		if err != nil {
			dbg.Fatal(err)
		}
		sims[i] = sim
		if host.Entity.Id == sc.Tree.Root.Entity.Id {
			dbg.Lvl2(hostAddress, "is root-node, will start protocol")
			rootSim = sim
			rootSC = sc
		}
	}
	if rootSim != nil {
		// If this cothority has the root-host, it will start the simulation
		dbg.Lvl2("Starting protocol", simul, "on host", rootSC.Host.Entity.Addresses)
		//dbg.Lvl5("Tree is", rootSC.Tree.Dump())

		// First count the number of available children
		childrenWait := monitor.NewMeasure("ChildrenWait")
		wait := true
		// The timeout starts with 1 second, which is the time of response between
		// each level of the tree.
		timeout := 1000
		for wait {
			node, err := rootSC.Overlay.CreateNewNodeName("Count", rootSC.Tree)
			if err != nil {
				dbg.Fatal(err)
			}
			node.ProtocolInstance().(*manage.ProtocolCount).SetTimeout(timeout)
			node.Start()
			dbg.Lvl1("Started counting children with timeout of", timeout)
			select {
			case count := <-node.ProtocolInstance().(*manage.ProtocolCount).Count:
				if count == rootSC.Tree.Size() {
					dbg.Lvl1("Found all", count, "children")
					wait = false
				} else {
					dbg.Lvl1("Found only", count, "children, counting again")
				}
				//case <-time.After(time.Millisecond * time.Duration(timeout) * 2):
				//	// Wait longer than the root-node before aborting
				//	dbg.Lvl1("Timed out waiting for children")
			}
			// Double the timeout and try again if not successful.
			timeout *= 2
		}
		childrenWait.Measure()
		dbg.Lvl1("Starting new node", simul)
		err := rootSim.Run(rootSC)
		if err != nil {
			dbg.Fatal(err)
		}

		// Test if all Entities are used in the tree, else we'll run into
		// troubles with CloseAll
		if !rootSC.Tree.UsesList() {
			dbg.Error("The tree doesn't use all Entities from the list!\n" +
				"This means that the CloseAll will fail and the experiment never ends!")
		}
		if rootSC.GetSingleHost() {
			// In case of "SingleHost" we need a new tree that contains every
			// entity only once, whereas rootSC.Tree will have the same
			// entity at different TreeNodes, which makes it difficult to
			// correctly close everything.
			dbg.Lvl2("Making new root-tree for SingleHost config")
			closeTree := rootSC.EntityList.GenerateBinaryTree()
			rootSC.Overlay.RegisterTree(closeTree)
			_, err = rootSC.Overlay.StartNewNodeName("CloseAll", closeTree)
		} else {
			_, err = rootSC.Overlay.StartNewNodeName("CloseAll", rootSC.Tree)
		}
		monitor.EndAndCleanup()
		if err != nil {
			dbg.Fatal(err)
		}
	}

	// Wait for all hosts to be closed
	allClosed := make(chan bool)
	go func() {
		for _, sc := range scs {
			sc.Host.WaitForClose()
			dbg.Lvl3(hostAddress, "Simulation closed host", sc.Host.Entity.Addresses, "closed")
		}
		allClosed <- true
	}()
	dbg.Lvl3(hostAddress, scs[0].Host.Entity.First(), "is waiting for all hosts to close")
	select {
	case <-allClosed:
		dbg.Lvl2(hostAddress, ": all hosts closed")
	case <-time.After(time.Second * time.Duration(scs[0].GetCloseWait())):
		dbg.Lvl2(hostAddress, ": didn't close after", scs[0].GetCloseWait(), " seconds")
	}
}
