// deter is the deterlab process that should run on the boss node
//
// It spawns multiple timestampers and clients, while constructing
// the topology defined on tree.json. It assumes that hosts.txt has
// the entire list of hosts to run timestampers on and that the final
// host is the designated logging server.
//
// The overall topology that is created is defined by tree.json.
// The port layout for each node, however, is specified here.
// tree.json will assign each node a port p. This is the port
// that each singing node is listening on. The timestamp server
// to which clients connect is listneing on port p+1. And the
// pprof server for each node is listening on port p+2. This
// means that in order to debug each client, you can forward
// the p+2 port of each node to your localhost.
//
// In the future the loggingserver will be connecting to the
// servers on the pprof port in order to gather extra data.
package main

import (
	"flag"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dedis/cothority/deploy/platform"
	"github.com/dedis/cothority/lib/cliutils"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/monitor"
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

var deterlab platform.Deterlab
var kill = false

func init() {
	flag.BoolVar(&kill, "kill", false, "kill everything (and don't start anything)")
}

func main() {
	deterlab.ReadConfig()
	flag.Parse()

	vpmap := make(map[string]string)
	for i := range deterlab.Virt {
		vpmap[deterlab.Virt[i]] = deterlab.Phys[i]
	}
	// kill old processes
	var wg sync.WaitGroup
	re := regexp.MustCompile(" +")
	hosts, err := exec.Command("/usr/testbed/bin/node_list", "-e", deterlab.Project+","+deterlab.Experiment).Output()
	if err != nil {
		dbg.Fatal("Deterlab experiment", deterlab.Project+"/"+deterlab.Experiment, "seems not to be swapped in. Aborting.")
		os.Exit(-1)
	}
	hosts_trimmed := strings.TrimSpace(re.ReplaceAllString(string(hosts), " "))
	hostlist := strings.Split(hosts_trimmed, " ")
	doneHosts := make([]bool, len(hostlist))
	dbg.Lvl2("Found the following hosts:", hostlist)
	if kill {
		dbg.Lvl1("Cleaning up", len(hostlist), "hosts.")
	}
	for i, h := range hostlist {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			if kill {
				dbg.Lvl4("Cleaning up host", h, ".")
				cliutils.SshRun("", h, "sudo killall -9 "+deterlab.App+" logserver forkexec timeclient scp 2>/dev/null >/dev/null")
				time.Sleep(1 * time.Second)
				cliutils.SshRun("", h, "sudo killall -9 "+deterlab.App+" 2>/dev/null >/dev/null")
				time.Sleep(1 * time.Second)
				// Also kill all other process that start with "./" and are probably
				// locally started processes
				cliutils.SshRun("", h, "sudo pkill -9 -f '\\./'")
				time.Sleep(1 * time.Second)
				if dbg.DebugVisible > 3 {
					dbg.Lvl4("Cleaning report:")
					cliutils.SshRunStdout("", h, "ps aux")
				}
			} else {
				dbg.Lvl3("Setting the file-limit higher on", h)

				// Copy configuration file to make higher file-limits
				err := cliutils.SshRunStdout("", h, "sudo cp remote/cothority.conf /etc/security/limits.d")
				if err != nil {
					dbg.Fatal("Couldn't copy limit-file:", err)
				}
			}
			doneHosts[i] = true
			dbg.Lvl3("Host", h, "cleaned up")
		}(i, h)
	}

	cleanupChannel := make(chan string)
	go func() {
		wg.Wait()
		dbg.Lvl3("Done waiting")
		cleanupChannel <- "done"
	}()
	select {
	case msg := <-cleanupChannel:
		dbg.Lvl3("Received msg from cleanupChannel", msg)
	case <-time.After(time.Second * 20):
		for i, m := range doneHosts {
			if !m {
				dbg.Lvl1("Missing host:", hostlist[i], "- You should run")
				dbg.Lvl1("/usr/testbed/bin/node_reboot", hostlist[i])
			}
		}
		dbg.Fatal("Didn't receive all replies while cleaning up - aborting.")
	}

	if kill {
		dbg.Lvl2("Only cleaning up - returning")
		return
	}

	// ADDITIONS : the monitoring part
	// Proxy will listen on Sink:SinkPort and redirect every packet to
	// RedirectionAddress:SinkPort-1. With remote tunnel forwarding it will
	// be forwarded to the real sink
	proxyAddress := deterlab.ProxyAddress + ":" + strconv.Itoa(monitor.SinkPort+1)
	dbg.Lvl2("Launching proxy redirecting to", proxyAddress)
	err = monitor.Proxy(proxyAddress)
	if err != nil {
		dbg.Fatal("Couldn't start proxy:", err)
	}

	hostnames := deterlab.Hostnames
	dbg.Lvl4("hostnames:", hostnames)

	// mapping from physical node name to the timestamp servers that are running there
	// essentially a reverse mapping of vpmap except ports are also used
	physToServer := make(map[string][]string)
	for _, virt := range hostnames {
		v, _, _ := net.SplitHostPort(virt)
		p := vpmap[v]
		ss := physToServer[p]
		ss = append(ss, virt)
		physToServer[p] = ss
	}

	monitorAddr := deterlab.MonitorAddress + ":" + strconv.Itoa(monitor.SinkPort)
	servers := len(physToServer)
	ppm := len(deterlab.Hostnames) / servers
	dbg.Lvl1("starting", servers, "forkexecs with", ppm, "processes each =", servers*ppm)
	totalServers := 0
	for phys, virts := range physToServer {
		if len(virts) == 0 {
			continue
		}
		totalServers += len(virts)
		dbg.Lvl2("Launching forkexec for", len(virts), "clients on", phys)
		wg.Add(1)
		go func(phys string) {
			//dbg.Lvl4("running on", phys, cmd)
			defer wg.Done()
			dbg.Lvl4("Starting servers on physical machine ", phys, "with monitor = ",
				deterlab.MonitorAddress, ":", monitor.SinkPort)
			err := cliutils.SshRunStdout("", phys, "cd remote; sudo ./forkexec"+
				" -physaddr="+phys+" -monitor="+monitorAddr)
			if err != nil {
				dbg.Lvl1("Error starting timestamper:", err, phys)
			}
			dbg.Lvl4("Finished with Timestamper", phys)
		}(phys)
	}

	if deterlab.App == "stamp" || deterlab.App == "sign" {
		// Every stampserver that started up (mostly waiting for configuration-reading)
		// writes its name in coll_stamp_dir - once everybody is there, the directory
		// is cleaned to flag it's OK to go on.
		start_config := time.Now()
		for {
			s, err := monitor.GetReady(monitorAddr)
			if err != nil {
				dbg.Fatal("Couldn't contact monitor at", monitorAddr)
			} else {
				dbg.Lvl1("Processes started:", s.Ready, "/", totalServers, "after", time.Since(start_config))
				if s.Ready == totalServers {
					dbg.Lvl2("Everybody ready, starting")
					// 1st second for everybody to see the deleted directory
					// 2nd second for everybody to start up listening
					time.Sleep(time.Second * 2)
					break
				}
			}
			time.Sleep(time.Second)
		}
	}

	switch deterlab.App {
	case "stamp":
		dbg.Lvl1("starting", len(physToServer), "time clients")
		// start up one timeclient per physical machine
		// it requests timestamps from all the servers on that machine
		amroot := true
		for p, ss := range physToServer {
			if len(ss) == 0 {
				dbg.Lvl3("ss is empty - not starting")
				continue
			}
			servers := strings.Join(ss, ",")
			dbg.Lvl3("Starting with ss=", ss)
			go func(p string, a bool) {
				cmdstr := "cd remote; sudo ./" + deterlab.App + " -mode=client " +
					" -name=client@" + p +
					" -server=" + servers +
					" -amroot=" + strconv.FormatBool(a)
				dbg.Lvl3("Users will launch client:", cmdstr)
				err := cliutils.SshRunStdout("", p, cmdstr)
				if err != nil {
					dbg.Lvl4("Deter.go: error for", deterlab.App, err)
				}
				dbg.Lvl4("Deter.go: Finished with", deterlab.App, p)
			}(p, amroot)
			amroot = false
		}
	case "sign_no":
		// TODO: for now it's only a simple startup from the server
		dbg.Lvl1("Starting only one client")
	}

	// wait for the servers to finish before stopping
	wg.Wait()
}
