package randhound

import (
	"log"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dedis/cothority/lib/sda"
)

func init() {
	sda.SimulationRegister("RandHound", NewRHSimulation)
}

// RHSimulation implements a RandHound simulation
type RHSimulation struct {
	sda.SimulationBFTree
	Trustees uint32
	Purpose  string
	Shards   uint32
}

// NewRHSimulation creates a new RandHound simulation
func NewRHSimulation(config string) (sda.Simulation, error) {
	rhs := new(RHSimulation)
	_, err := toml.Decode(config, rhs)
	if err != nil {
		return nil, err
	}
	return rhs, nil
}

// Setup configures a RandHound simulation with certain parameters
func (rhs *RHSimulation) Setup(dir string, hosts []string) (*sda.SimulationConfig, error) {
	sim := new(sda.SimulationConfig)
	rhs.Hosts = len(hosts)
	rhs.CreateEntityList(sim, hosts, 2000)
	err := rhs.CreateTree(sim)
	if err != nil {
		return nil, err
	}
	return sim, nil
}

// Run initiates a RandHound simulation
func (rhs *RHSimulation) Run(config *sda.SimulationConfig) error {
	leader, err := config.Overlay.CreateNewNode("RandHound", config.Tree)
	if err != nil {
		return err
	}
	rh := leader.ProtocolInstance().(*RandHound)
	err = rh.Setup(uint32(rhs.Hosts), rhs.Trustees, rhs.Purpose)
	if err != nil {
		return err
	}
	log.Printf("RandHound - group config: %d %d %d %d %d %d\n", rh.Group.N, rh.Group.F, rh.Group.L, rh.Group.K, rh.Group.R, rh.Group.T)
	log.Printf("RandHound - shards: %d\n", rhs.Shards)
	rh.StartProtocol()

	select {
	case <-rh.Leader.Done:
		log.Printf("RandHound - done")
		rnd, err := rh.Random()
		if err != nil {
			panic(err)
		}
		sharding, err := rh.Shard(rnd, rhs.Shards)
		if err != nil {
			panic(err)
		}
		log.Printf("RandHound - random bytes: %v\n", rnd)
		log.Printf("RandHound - sharding: %v\n", sharding)
	case <-time.After(time.Second * 60):
		log.Printf("RandHound - time out")
	}

	return nil

}