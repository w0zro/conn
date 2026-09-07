package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// A project on docker runs its services in containers, and a container is
// not a process the scan can see: on macOS it runs in docker's own machine,
// and on Linux it is root's, working in a directory of its own. docker
// knows, and compose writes on every container it starts the directory
// it was started for — the same fact lsof reports of a process — so a
// container is filed under a place by the rule everything else is. It is
// listed as a row named for its service, with the ports it publishes on
// the host, under the docker compose up that runs it when one runs in a
// shell and under the place itself when compose was left detached. Its
// row has no shell to enter; x asks docker to stop it.

// Container is what docker said of one container, kept on the Proc that
// stands for it in the tree.
type Container struct {
	ID      string // the short id, twelve hex digits
	Name    string // the container's name: compose-demo-web-1
	Service string // the compose service, or the name when compose did not start it
	Image   string
	Status  string // Up 3 minutes
}

// dockerPath is where the docker client is, or "" where there is none: a
// machine without docker is asked nothing, every scan.
var dockerPath = func() string {
	p, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	return p
}()

// containerLabels are the labels compose writes that say where a
// container belongs and what it is called.
const (
	labelWorkingDir = "com.docker.compose.project.working_dir"
	labelService    = "com.docker.compose.service"
)

// containers lists the running containers as rows for the tree: only the
// ones compose started for a directory, since a container without one
// belongs to no place, the way a process working outside every project
// belongs to none. A daemon that is not up, or no docker at all, is an
// empty list — nothing is running in a container, which is true.
func containers() []Proc {
	if dockerPath == "" {
		return nil
	}
	out, err := listing(scanTimeout, dockerPath, "ps", "--format", "{{json .}}")
	if err != nil {
		return nil
	}
	return parseContainers(out)
}

// dockerRow is the shape of one line of docker ps --format '{{json .}}':
// the fields conn reads, and the labels and ports as the strings docker
// prints them.
type dockerRow struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	Labels string `json:"Labels"`
	Ports  string `json:"Ports"`
}

// parseContainers reads what docker ps said, one container a line.
func parseContainers(out []byte) []Proc {
	var procs []Proc
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row dockerRow
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil || row.ID == "" {
			continue
		}
		labels := parseLabels(row.Labels)
		dir := labels[labelWorkingDir]
		if dir == "" {
			continue
		}
		service := labels[labelService]
		if service == "" {
			service = row.Names
		}
		procs = append(procs, Proc{
			PID:     containerPID(row.ID),
			Command: service,
			Dir:     dir,
			Ports:   hostPorts(row.Ports),
			Container: &Container{
				ID: row.ID, Name: row.Names, Service: service,
				Image: row.Image, Status: row.Status,
			},
		})
	}
	return procs
}

// parseLabels reads labels as docker ps prints them: key=value, comma
// separated. A value can hold a comma of its own — a maintainer's address
// — so a piece with no = in it is the tail of the value before it.
func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	last := ""
	for piece := range strings.SplitSeq(s, ",") {
		key, value, ok := strings.Cut(piece, "=")
		if !ok {
			if last != "" {
				labels[last] += "," + piece
			}
			continue
		}
		labels[key] = value
		last = key
	}
	return labels
}

// hostPorts is the ports a container publishes on the host, as docker ps
// prints them — 0.0.0.0:8438->80/tcp, [::]:8438->80/tcp — each once,
// lowest first. A port with no host side is the container's own and is
// nowhere to go.
func hostPorts(s string) []string {
	var ports []string
	for piece := range strings.SplitSeq(s, ",") {
		host, _, ok := strings.Cut(strings.TrimSpace(piece), "->")
		if !ok {
			continue
		}
		if port, ok := portOf(host); ok && !slices.Contains(ports, port) {
			ports = append(ports, port)
		}
	}
	sortPorts(ports)
	return ports
}

// containerPID is the number a container goes by in the tree, where every
// node has one: below zero, where no process is, and the same for the
// container from one scan to the next, so the cursor holds its row. It is
// read off the id, whose first digits tell containers apart the way the
// whole id does.
func containerPID(id string) int {
	v, err := strconv.ParseInt(id[:min(6, len(id))], 16, 64)
	if err != nil {
		v = 0
	}
	return -int(v) - 2
}

// attachContainers files each container under the compose that runs it,
// when one is running in a shell: the process working in the container's
// directory that is compose, the deepest of them — compose is a docker
// running its plugin, and the plugin is the one that holds the containers.
// With no compose running there — up -d, and the shell gone — a container
// is a root of its own under the place, which is where its directory says
// it belongs.
func attachContainers(procs, cs []Proc) []Proc {
	for i := range cs {
		cs[i].PPID = composeFor(procs, cs[i].Dir)
	}
	return append(procs, cs...)
}

// composeFor is the pid of the deepest compose process working in dir, or
// zero.
func composeFor(procs []Proc, dir string) int {
	isCompose := map[int]bool{}
	for _, p := range procs {
		if p.Dir == dir && p.Container == nil && runsCompose(p) {
			isCompose[p.PID] = true
		}
	}
	for _, p := range procs {
		if isCompose[p.PID] && !isCompose[p.PPID] {
			// The top of a compose run; walk to its bottom.
			pid := p.PID
			for {
				next := 0
				for _, c := range procs {
					if c.PPID == pid && isCompose[c.PID] {
						next = c.PID
						break
					}
				}
				if next == 0 {
					return pid
				}
				pid = next
			}
		}
	}
	return 0
}

// runsCompose reports a process that is compose: docker running its
// compose command, or the compose plugin itself.
func runsCompose(p Proc) bool {
	fields := strings.Fields(p.Argv)
	if len(fields) < 2 {
		return false
	}
	name := strings.TrimPrefix(fieldBase(fields[0]), "docker-")
	return (name == "docker" || name == "compose") && slices.Contains(fields[1:], "compose")
}

// fieldBase is the last element of a path, or the word itself.
func fieldBase(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// dockerStop asks docker to stop a container: the container's own
// process gets SIGTERM, and docker follows with SIGKILL after its grace,
// which is the kill a process gets, done docker's way.
func dockerStop(id string) error {
	_, err := listing(scanTimeout, dockerPath, "stop", id)
	return err
}

// containerLogs is the last of what a container has written, both
// streams, for the pane.
func containerLogs(id string, lines int) []string {
	if dockerPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, dockerPath, "logs", "--tail", strconv.Itoa(lines), id).CombinedOutput()
	said := strings.TrimRight(string(out), "\n")
	if said == "" {
		return nil
	}
	return strings.Split(said, "\n")
}

// containerFields describes a container the way procFields describes a
// process: what it is, where it belongs, and what docker says of it,
// then the last of its logs.
func containerFields(n *ProcNode) []field {
	c := n.Container
	fs := []field{
		heading(procLabel(n)),
		note(n.Dir),
		gap(),
		field{label: "container", value: c.Name, tone: toneName},
		field{label: "image", value: c.Image, tone: toneQuiet},
		field{label: "status", value: c.Status, tone: toneGood},
	}
	if len(n.Ports) > 0 {
		fs = append(fs, field{label: "publishes", value: strings.Join(n.Ports, ", "), tone: toneAccent})
	}
	return append(fs, transcript(containerLogs(c.ID, transcriptLines))...)
}
