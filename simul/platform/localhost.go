package platform

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/dedis/cothority/lib/cliutils"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/monitor"
	"github.com/dedis/cothority/lib/sda"
	_ "github.com/dedis/cothority/protocols"
	"strings"
)

// Localhost is responsible for launching the app with the specified number of nodes
// directly on your machine, for local testing.

// Localhost is the platform for launching thee apps locally
type Localhost struct {

	// Address of the logger (can be local or not)
	logger string

	// The simulation to run
	Simulation string

	// Where is the Localhost package located
	localDir string
	// Where to build the executables +
	// where to read the config file
	// it will be assembled like LocalDir/RunDir
	runDir string

	// Debug level 1 - 5
	debug int

	// The number of servers
	servers int
	// All addresses - we use 'localhost1'..'localhostn' to
	// identify the different cothorities, but when opening the
	// ports they will be converted to normal 'localhost'
	addresses []string

	// Whether we started a simulation
	running bool
	// WaitGroup for running processes
	wg_run sync.WaitGroup

	// errors go here:
	errChan chan error

	// SimulationConfig holds all things necessary for the run
	sc *sda.SimulationConfig
}

// Configure various
func (d *Localhost) Configure(pc *PlatformConfig) {
	pwd, _ := os.Getwd()
	d.runDir = pwd + "/platform/localhost"
	d.localDir = pwd
	d.debug = pc.Debug
	d.running = false
	d.errChan = make(chan error)
	if d.Simulation == "" {
		dbg.Fatal("No simulation defined in simulation")
	}
	dbg.Lvl3(fmt.Sprintf("Localhost dirs: RunDir %s", d.runDir))
	dbg.Lvl3("Localhost configured ...")
}

// Will build the application
func (d *Localhost) Build(build string, arg ...string) error {
	src := "./cothority"
	dst := d.runDir + "/" + d.Simulation
	start := time.Now()
	// build for the local machine
	res, err := cliutils.Build(src, dst,
		runtime.GOARCH, runtime.GOOS,
		arg...)
	if err != nil {
		dbg.Fatal("Error while building for localhost (src", src, ", dst", dst, ":", res)
	}
	dbg.Lvl3("Localhost: Build src", src, ", dst", dst)
	dbg.Lvl4("Localhost: Results of localhost build:", res)
	dbg.Lvl2("Localhost: build finished in", time.Since(start))
	return err
}

func (d *Localhost) Cleanup() error {
	dbg.Lvl3("Cleaning up")
	ex := d.runDir + "/" + d.Simulation
	err := exec.Command("pkill", "-f", ex).Run()
	if err != nil {
		dbg.Lvl3("Error stopping localhost", err)
	}

	// Wait for eventual connections to clean up
	time.Sleep(time.Second)
	return nil
}

func (d *Localhost) Deploy(rc RunConfig) error {
	if runtime.GOOS == "darwin" {
		files, err := exec.Command("ulimit", "-n").Output()
		if err != nil {
			dbg.Fatal("Couldn't check for file-limit:", err)
		}
		filesNbr, err := strconv.Atoi(strings.TrimSpace(string(files)))
		if err != nil {
			dbg.Fatal("Couldn't convert", files, "to a number:", err)
		}
		hosts, _ := strconv.Atoi(rc.Get("hosts"))
		if filesNbr < hosts*2 {
			maxfiles := 10000 + hosts*2
			dbg.Fatalf("Maximum open files is too small. Please run the following command:\n"+
				"sudo sysctl -w kern.maxfiles=%d\n"+
				"sudo sysctl -w kern.maxfilesperproc=%d\n"+
				"ulimit -n %d\n"+
				"sudo sysctl -w kern.ipc.somaxconn=2048\n",
				maxfiles, maxfiles, maxfiles)
		}
	}

	d.servers, _ = strconv.Atoi(rc.Get("servers"))
	dbg.Lvl2("Localhost: Deploying and writing config-files for", d.servers, "servers")
	sim, err := sda.NewSimulation(d.Simulation, string(rc.Toml()))
	if err != nil {
		return err
	}
	d.addresses = make([]string, d.servers)
	for i := range d.addresses {
		d.addresses[i] = "localhost" + strconv.Itoa(i)
	}
	d.sc, err = sim.Setup(d.runDir, d.addresses)
	if err != nil {
		return err
	}
	d.sc.Config = string(rc.Toml())
	d.sc.Save(d.runDir)
	dbg.Lvl2("Localhost: Done deploying")
	return nil

}

func (d *Localhost) Start(args ...string) error {
	os.Chdir(d.runDir)
	dbg.Lvl4("Localhost: chdir into", d.runDir)
	ex := d.runDir + "/" + d.Simulation
	d.running = true
	dbg.Lvl1("Starting", d.servers, "applications of", ex)
	for index := 0; index < d.servers; index++ {
		d.wg_run.Add(1)
		dbg.Lvl3("Starting", index)
		host := "localhost" + strconv.Itoa(index)
		cmdArgs := []string{"-address", host, "-monitor",
			"localhost:" + strconv.Itoa(monitor.DefaultSinkPort),
			"-simul", d.Simulation,
			"-debug", strconv.Itoa(dbg.DebugVisible()),
		}
		cmdArgs = append(args, cmdArgs...)
		dbg.Lvl3("CmdArgs are", cmdArgs)
		cmd := exec.Command(ex, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		go func(i int, h string) {
			dbg.Lvl3("Localhost: will start host", h)
			err := cmd.Run()
			if err != nil {
				dbg.Error("Error running localhost", h, ":", err)
				d.errChan <- err
			}
			d.wg_run.Done()
			dbg.Lvl3("host (index", i, ")", h, "done")
		}(index, host)
	}
	return nil
}

// Waits for all processes to finish
func (d *Localhost) Wait() error {
	dbg.Lvl3("Waiting for processes to finish")

	var err error
	go func() {
		d.wg_run.Wait()
		dbg.Lvl3("WaitGroup is 0")
		// write to error channel when done:
		d.errChan <- nil
	}()

	// if one of the hosts fails, stop waiting and return the error:
	select {
	case e := <-d.errChan:
		dbg.Lvl3("Finished waiting for hosts:", e)
		if e != nil {
			d.Cleanup()
			err = e
		}
	}

	dbg.Lvl2("Processes finished")
	return err
}
