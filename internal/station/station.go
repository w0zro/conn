// Package station reads the station conn comes up on: the machine, who
// is at it, the build of conn running there, and the directory conn
// keeps its state in. It reads and puts no word to any of it; the
// console words it.
package station

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
	Tools   []Tool // what conn needs on this platform past the kernel
}

// Read reads the station; stamp is what the build stamped on the
// binary, which is what git describe said, or nothing. Nothing here
// waits on the network; the programs it runs answer from disk and are
// given a moment each.
func Read(stamp string) Station {
	st := Station{Build: ReadBuild(stamp), Login: ReadLogin()}
	st.Machine = readMachine()
	st.Volume = readVolume(st.Login.Home)
	st.Network, st.NetRead = readNetwork()
	st.State = readStateDir(st.Login.Home)
	st.Tools = readTools()
	return st
}
