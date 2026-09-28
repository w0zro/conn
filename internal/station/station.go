// Package station reads the station conn comes up on: the machine, who
// is at it, the build of conn running there, and the directories conn
// keeps. It reads and puts no word to any of it; the console words it.
package station

import "github.com/w0zro/conn/internal/config"

// A Station is everything conn reads as it comes up, before a word is
// put to any of it: the machine, the session, the build, the volume
// under home, the network, and the state directory. Read reads it; the
// console words it. Keeping the two apart is what lets the words be
// tested against a station on file, and re-said as the clock turns.
type Station struct {
	Machine Machine
	Login   Login
	Build   Build
	Volume  Volume
	Network Network
	NetRead bool
	State   StateDir
	Config  config.State
	Tools   []Tool // what the platform needs past the kernel
}

// Read reads the station. Nothing here waits on the network; the
// programs it runs answer from disk and are given a moment each.
func Read() Station {
	st := Station{Build: ReadBuild(), Login: ReadLogin()}
	st.Machine = readMachine()
	st.Volume = readVolume(st.Login.Home)
	st.Network, st.NetRead = readNetwork()
	st.State = readStateDir(st.Login.Home)
	st.Config = config.ReadState(st.Login.Home)
	st.Tools = readTools()
	return st
}
