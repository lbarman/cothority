/*
 * Time-measurement functions.
 *
 * Usage:
 * ```measure := monitor.NewMeasure()```
 * ```// Do some calculations```
 * ```measure.MeasureWall("CPU on calculations")```
 */

package monitor

import (
	"encoding/json"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/dedis/cothority/lib/dbg"
)

// Structs are encoded through a json encoder.
var encoder *json.Encoder
var connection net.Conn

// ConnectSink connects to the given endpoint and initialises a json
// encoder. It can be the address of a proxy or a monitoring process.
// Returns an error if it could not connect to the endpoint.
func ConnectSink(addr string) error {
	if encoder != nil {
		return nil
	}
	dbg.Lvl3("Connecting to:", addr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	dbg.Lvl3("Connected to sink:", addr)
	connection = conn
	encoder = json.NewEncoder(conn)
	return nil
}

// Only sends a ready-string
func Ready(addr string) error {
	if encoder == nil {
		dbg.Lvl3("Connecting to sink", addr)
		err := ConnectSink(addr)
		if err != nil {
			return err
		}
	}
	dbg.Lvl3("Sending ready-signal")
	send(Measure{Name: "ready"})
	return nil
}

// Returns how many peers are ready
func GetReady(addr string) (*Stats, error) {
	if encoder == nil {
		err := ConnectSink(addr)
		if err != nil {
			return nil, err
		}
	}
	dbg.Lvl3("Getting ready_count")
	send(Measure{Name: "ready_count"})
	decoder := json.NewDecoder(connection)
	var s Stats
	err := decoder.Decode(&s)
	if err != nil {
		return nil, err
	}
	dbg.Lvlf3("Received stats with %+v", s)
	return &s, nil
}

// Send transmits the given struct over the network.
func send(v interface{}) {
	if encoder == nil {
		panic(fmt.Errorf("Monitor's sink connection not initalized. Can not send any measures"))
	}

	// For a large number of clients (˜10'000), the connection phase
	// can take some time. This is a linear backoff to enable connection
	// even when there are a lot of request:
	for wait := 500; wait < 1000; wait += 100 {
		if err := encoder.Encode(v); err == nil {
			return
		} else {
			dbg.Lvl1("Couldn't send to monitor-sink:", err)
			time.Sleep(time.Duration(wait) * time.Millisecond)
		}
	}
	panic(fmt.Errorf("No contact to monitor-sink possible!"))
}

// Measure holds the different values that can be computed for a measure.
// Measures are sent for further processing from the client to the monitor.
type Measure struct {
	Name        string
	WallTime    float64
	CPUTimeUser float64
	CPUTimeSys  float64
	// These are used for communicating with the clients
	Sender string
	Ready  int
	// Since we send absolute timing values, we need to store our reference too.
	lastWallTime time.Time
	autoReset    bool
}

// NewMeasure creates a new measure struct and enables automatic reset after
// each Measure call.
func NewMeasure(name string) *Measure {
	m := &Measure{Name: name}
	m.enableAutoReset(true)
	return m
}

// Takes a measure, sends it to the monitor and resets all timers.
func (m *Measure) Measure() {
	// Wall time measurement
	m.WallTime = float64(time.Since(m.lastWallTime)) / 1.0e9
	// CPU time measurement
	m.CPUTimeSys, m.CPUTimeUser = getDiffRTime(m.CPUTimeSys, m.CPUTimeUser)
	// send data
	send(m)
	// reset timers
	m.reset()
}

// Enables / Disables automatic reset of a measure. If called with true, the
// measure is reset.
func (m *Measure) enableAutoReset(b bool) {
	m.autoReset = b
	m.reset()
}

// Resets the timers in a measure to 'now'.
func (m *Measure) reset() {
	if m.autoReset {
		m.CPUTimeSys, m.CPUTimeUser = GetRTime()
		m.lastWallTime = time.Now()
	}
}

// Prints a message to end the logging.
func EndAndCleanup() {
	send(Measure{Name: "end"})
	if err := connection.Close(); err != nil {
		dbg.Error("Could not close connection:", err)
	} else {
		dbg.Lvl3("Closed connection:", connection)
		encoder = nil
	}
}

// Converts microseconds to seconds.
func iiToF(sec int64, usec int64) float64 {
	return float64(sec) + float64(usec)/1000000.0
}

// Returns the sytem and the user time so far.
func GetRTime() (tSys, tUsr float64) {
	rusage := &syscall.Rusage{}
	syscall.Getrusage(syscall.RUSAGE_SELF, rusage)
	s, u := rusage.Stime, rusage.Utime
	return iiToF(int64(s.Sec), int64(s.Usec)), iiToF(int64(u.Sec), int64(u.Usec))
}

// Returns the difference of the given system- and user-time.
func getDiffRTime(tSys, tUsr float64) (tDiffSys, tDiffUsr float64) {
	nowSys, nowUsr := GetRTime()
	return nowSys - tSys, nowUsr - tUsr
}
